# Performance

SysSentient runs continuously on machines it is meant to observe, not disturb.
Its own cost is therefore a correctness concern, not a nicety.

## Measured

Fedora workstation, 8 cores, 594 processes, default 2-second poll.

| | Before | After |
|---|---|---|
| Idle CPU | **4.10%** of one core | **0.78%** |
| Resident memory | 34.1 MB | 26.1 MB |
| `Collect()` wall time | 566 ms | 20.5 ms |
| `Collect()` allocations | 35,957 | 6,686 |
| `Collect()` garbage | 14.8 MB | 0.78 MB |
| `getTopProcesses()` garbage | 13.9 MB | 0.21 MB |

For reference, `node_exporter` idles under 1% on comparable hardware.

## Reproducing the measurement

Both halves matter: the benchmark isolates the collector, and the process
measurement catches everything else — storage, the WebSocket hub, HTTP.

### Benchmarks

```bash
go test ./internal/collector/ -run '^$' -bench 'BenchmarkCollect|BenchmarkGetTopProcesses' -benchtime=20x -count=3
go test ./internal/storage/   -run '^$' -bench 'BenchmarkSave|BenchmarkGetRecent'          -benchtime=500x
```

Use `-count=3` and read the middle result. A single run of five iterations is
dominated by scheduling noise — an early attempt at this work appeared to make
`getTopProcesses` *slower* purely because of that.

### Idle CPU of the running daemon

Match on the executable name, not the command line. `pgrep -f` matches the
measuring shell's own arguments and will happily report 0.00% for a process
that is not the daemon.

```bash
PID=$(pgrep -x sys-daemon | head -1)
T1=$(awk '{print $14+$15}' /proc/$PID/stat); sleep 40; T2=$(awk '{print $14+$15}' /proc/$PID/stat)
awk -v a="$T1" -v b="$T2" 'BEGIN{printf "idle CPU: %.2f%%\n", (b-a)/100/40*100}'
awk '/VmRSS/{print "RSS:", $2, $3}' /proc/$PID/status
```

Let the daemon settle for ~10 seconds first; start-up costs distort a short
window.

## Where the cost was

**A 500 ms sleep per tick.** `cpu.Percent(500*time.Millisecond, …)` blocked the
daemon's main select loop for a quarter of every two-second cycle. Go tickers
drop ticks rather than queue them, so any overrun silently lost samples. Since
the loop already polls every two seconds, passing an interval of `0` diffs
against the previous call and measures the same thing for free.

**gopsutil's `Times()`, 594 times per poll.** It builds a full `CPUTimesStat`
and allocates roughly fifty objects to answer a question that
`/proc/<pid>/stat` answers with two integers. That was ~57 ms and 13.5 MB of
garbage per collection — the single largest cost in the collector.

The replacement reads the file into a reusable buffer and scans for the two
fields it needs, keeping the whole 594-process sweep close to allocation-free.
It is Linux-only; Windows and macOS keep the gopsutil path, since that is not
where this runs at scale.

`proctimes_test.go` checks the fast reader against gopsutil on live processes
rather than assuming they agree — including the claim that the extra CPU fields
the old code summed are always zero per-process.

**Metadata for processes that were then discarded.** `Name`, `Username`,
`MemoryInfo` and `Status` were fetched for every process above a 0.1% threshold
and all but `limit` thrown away by the sort. Now ranking happens first and only
the survivors pay. On the measured host this was a small win — 19 processes
cleared the threshold, not hundreds — but it scales with how busy the machine
is, which is exactly when the cost is least welcome.

## Storage

Retention is tiered: full-resolution samples for 24 hours, per-minute rollups
for 30 days, per-five-minute for a year. Only the last tier deletes anything.

Measured on 1,800 samples (one hour at the default interval) with realistic
payloads — eight cores, ten processes, four filesystems:

| | |
|---|---|
| Raw row | 2,166 bytes |
| Raw, one hour | 3,808 KB |
| Same hour rolled up and vacuumed | **88 KB — 2.3% of raw** |
| Extrapolated raw, per host per day | 89.2 MB |

The raw tier still dominates the total, because a full sample carries JSON for
the per-core list, the process list and every filesystem. Reducing that payload
is separate work; tiering is what makes keeping a year of history affordable at
all.

Two SQLite fixes came with it. The connection pool was capped at one, which
enabled WAL and then forfeited the concurrent reads that are the only reason to
use it. And ingest wrote one autocommit transaction per sample, so an agent's
60-sample flush was 60 fsyncs contending for a single write lock — batching it
into one transaction is 2.1x faster (6.4ms to 3.1ms for 60 samples).

### Measuring the footprint

Close the database before sizing it. An open database keeps pending deletes in
the WAL, so measuring while it is open counts the data twice and makes a rollup
look like it *grew* the file — which is exactly what the first attempt at this
measurement reported.

### Row payload

Measured by averaging `LENGTH(CAST(col AS BLOB))` per column over a real
database, which attributes bytes to the column that actually holds them rather
than guessing from the schema.

| Column | Before | After |
|---|---:|---:|
| `processes` | 1027 B | 1003 B |
| `filesystems` | 600 B | 543 B |
| `top_processes` | 250 B | **0 B** |
| `cpu_per_core` | 147 B | 144 B |
| everything else | 225 B | 219 B |
| **total** | **2249 B** | **1909 B** |

**15.1% smaller**, almost entirely from one change: `top_processes` was a
human-readable rendering of `processes` — the identical data, formatted — so it
is now derived on read. Consumers still see it populated; the AI prompt reads
it back out of storage, and an empty value would silently degrade analysis.

The filesystem columns barely moved, and that is worth recording. `free_bytes`
and `used_percent` *look* derivable from `total_bytes` and `used_bytes`, and an
earlier version of this change dropped them on that basis for a headline 19.8%.
It was wrong. On this machine:

| Mount | fstype | free reported | total − used |
|---|---|---:|---:|
| `/` | btrfs | 242,961,608,704 | 244,080,635,904 |
| `/boot` | ext4 | 1,360,130,048 | 1,484,279,808 |
| `/boot/efi` | vfat | 606,896,128 | 606,896,128 |

Reserved blocks mean the derivation holds only where nothing is reserved. The
encoding now drops a derived field **only when it actually matches** the
derivation, making the compaction lossless by construction rather than by
assumption — so it saves on vfat, tmpfs and overlay mounts and nothing on the
root filesystem.

`processes` is now 53% of a row and is not redundant: it is the per-sample
drill-down data the console shows. Reducing it further means storing less
history, not storing the same history more compactly.

## Still open

- Collection remains synchronous with the tick. It no longer blocks for half a
  second, but a slow disk can still delay a sample. Moving it off the loop is
  tracked in `docs/plans/05-collector-hot-path.md`.
