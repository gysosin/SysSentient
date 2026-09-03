# 05 — Get idle CPU under 1%

| | |
|---|---|
| **Phase** | 2 — Performance |
| **Depends on** | 01 |
| **Status** | **done** — 4.10% -> 0.78% idle CPU |

## Why

Measured baseline: **4.1% CPU** at idle on 8 cores, 34 MB RSS, 14 threads.
`node_exporter` idles under 1%.

`Collect()` opens with a hard `cpu.Percent(500*time.Millisecond, false)` and is
called synchronously from the ticker in `cmd/daemon/main.go`, so at the default
2s interval the loop spends **at least 25% of every cycle inside collection**.
Go tickers drop ticks rather than queue them, so any overrun silently loses
samples.

## Scope

- Move collection off the main select loop.
- Cache process handles between polls instead of re-resolving every PID.
- Replace the full sort in `getTopProcesses` with a bounded heap.
- Adaptive interval: back off when the machine is idle.

## Acceptance criteria

- Idle CPU **under 1%**, measured the same way as the baseline.
- No sample loss under load — verify with a `stress-ng` run.
- Collection duration is exported as a self-metric on `/metrics`.

## Verification

```bash
PID=$(pgrep -f sys-daemon | head -1)
T1=$(awk '{print $14+$15}' /proc/$PID/stat); sleep 30; T2=$(awk '{print $14+$15}' /proc/$PID/stat)
awk -v a=$T1 -v b=$T2 'BEGIN{printf "avg CPU: %.2f%%\n", (b-a)/100/30*100}'
```

---

Every shard must also pass the project gate before it is pushed:

```bash
GOTOOLCHAIN=auto go vet ./... && GOTOOLCHAIN=auto go test ./... -race
cd web && npm audit --audit-level=moderate && npm run typecheck && npm test && npm run build
```
