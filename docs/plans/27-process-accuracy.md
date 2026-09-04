# 27 — Make processes true

| | |
|---|---|
| **Phase** | 4 — Correctness |
| **Depends on** | — |
| **Status** | **done** — the numbers reconcile and the count is real |

## Why

The Processes screen presented a CPU-ranked sample of ten as an inventory of
the machine, in units that could not be reconciled with the gauge beside it.

Measured on a 626-process, 8-core host before this change:

| | Reported | Actual |
|---|---|---|
| Process count | **10** | 626 |
| Sum of process CPU | **445.7%** | system CPU was 92.4% |
| Top row | `186.86867019353735` | one process, 1.9 cores |

**The memory column was meaningless.** `getTopProcesses` filtered on
`cpu > 0.1` *before* reading memory, so an idle process holding 8 GB was
dropped before anything looked at it — while the UI offered a Memory column and
a memory sort over the ten CPU-active survivors.

Other faults: exited processes rendered as `Sleeping, 0 MB` with an empty name,
because metadata errors were discarded; `userHZ` was hardcoded to 100 and wrong
by 2.5× or 10× on a `CONFIG_HZ=250/1000` kernel; sub-megabyte processes read
`0 MB`; and the live host filter compared a host **id** against `hostname`, so
selecting a host matched nothing.

## What changed

- **Two rankings, unioned** — top N by CPU and top N by memory. Nothing is
  filtered out before memory is read.
- **`ProcessCount`** is collected, stored and displayed separately from the
  sample.
- **`CPU` is whole-machine percent**, comparable with the system gauge.
  **`CPUCore`** carries top's per-core figure alongside it.
- **`userHZ` read from `sysconf(_SC_CLK_TCK)`** instead of assumed.
- **Metadata failures drop the row** rather than rendering a dead process.
- **`MemoryBytes`** carries exact RSS; the UI formats KB/MB/GB.
- **`hostId` on every sample**, and the live filter uses it.

## Cost

Reading memory for every process instead of ten looked like it would cost
8.3 ms and 4,200 allocations per collection. `/proc/<pid>/stat` — already open
for CPU time — carries RSS in field 24, so the second file was unnecessary:

| | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| `main` (memory for 10) | 12,163,151 | 241,891 | 4,027 |
| this change (memory for all) | 12,000,935 | 454,901 | 5,458 |

**No measurable time cost.** Allocation rises because all ~600 candidates are
tracked rather than the ~19 that cleared the old CPU gate.

## Acceptance

Live, on the same 8-core host:

```
actual: 625 processes
reported: 623 processes | 17 rows kept

system cpu:      20.5 %
sum process cpu: 19.1 % (of machine)

name                 cpu%   core%      memory
chrome               12.5    99.9    2754.6MB
claude-desktop        1.6    12.6     845.8MB
```

The sums reconcile, the count is real, and one process saturating a core reads
12.5% of an eight-core machine and 99.9% of one core — both true, both shown.
