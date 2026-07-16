# Deployment guide — latency-critical configuration (P2-4)

This document describes the supported production configuration for running
`seq` with bounded dispatch latency and a starved garbage collector. It
assumes the Phase 2 work is in place: the hot path (frame read → parse →
publish → dispatch) allocates nothing in steady state.

## 1. Dispatch thread

The dispatch goroutine is pinned to one OS thread (`runtime.LockOSThread`)
unconditionally. Its idle behavior is configurable:

```yaml
node:
  dispatch:
    idle_strategy: gosched   # default; cooperative, correct on shared cores
    # idle_strategy: spin    # latency-critical; requires a dedicated core
    # spin_budget: 4096      # idle iterations between yields (spin only)
```

- `gosched`: the loop yields to the Go scheduler whenever the ring is empty.
  Use this whenever `seq` shares cores with other workloads.
- `spin`: the loop busy-spins `spin_budget` iterations between yields, keeping
  the thread and its cache hot for the next event. Only use with an isolated
  core (below); on a shared core it steals cycles without improving latency.

## 2. Core isolation (Linux)

Give the dispatch thread an isolated physical core so neither the kernel
scheduler nor other processes preempt it.

1. Isolate cores from the kernel scheduler at boot (e.g. cores 2–3):

```
# /etc/default/grub
GRUB_CMDLINE_LINUX="isolcpus=2,3 nohz_full=2,3 rcu_nocbs=2,3"
$ update-grub && reboot
```

2. Pin the process to the isolated cores:

```
$ taskset -c 2,3 ./seq -c config/prod.yml
```

The Go runtime multiplexes goroutines over the allowed cores; the locked
dispatch thread stays on one of them. For strict single-core placement of the
dispatch thread, run with `GOMAXPROCS=2` on two isolated cores: one core
serves the dispatch thread, the other absorbs the remaining goroutines
(adapter read loops, timers).

Also disable frequency scaling and deep C-states on the isolated cores
(`cpupower frequency-set -g performance`, `cpupower idle-set -D 0`) to avoid
wake-up latency after idle periods.

## 3. GOMAXPROCS

- Default (all cores) is fine for development.
- Under `taskset`, set `GOMAXPROCS` to the number of granted cores; the Go
  runtime does not always detect cpuset restrictions.
- Practical floor: `GOMAXPROCS >= 1 + number of WebSocket connections / 2`,
  and at least 2 — the dispatch thread must never share its P with the
  adapter read loops in the spin configuration.

Configurable via environment (`GOMAXPROCS=4`) or config:

```yaml
runtime:
  gomaxprocs: 4
```

## 4. Garbage collector: fuse, not periodic event

After Phase 2, steady-state allocation on the hot path is zero, so the GC has
nothing to collect at event rate. The supported production configuration
turns periodic GC off and uses the soft memory limit as a fuse against slow
leaks:

```yaml
runtime:
  gc_off: true                    # refused unless a memory limit is in force
  mem_limit_bytes: 4294967296     # 4 GiB GOMEMLIMIT
```

Environment equivalents (`GOGC=off`, `GOMEMLIMIT=4GiB`) work as well; config
wins when both are set. `gc_off` without any memory limit is refused at
startup.

If memory ever approaches the limit, the runtime runs a collection — the fuse
blowing is a signal to investigate an allocation regression (the P3-1 gates
should have caught it in CI first).

### Quiet-window collections

With GC off, operators may force a collection during declared quiet windows
(e.g. exchange maintenance) via the metrics server:

```
$ curl -X POST http://127.0.0.1:9100/gc
```

## 5. Metrics

```yaml
metrics:
  enabled: true
  addr: 127.0.0.1:9100
```

`GET /metrics` returns plain text:

- `seq_events_dropped_total{topic=...}` / `seq_events_overflow_waits_total`
  — the P0-1 overflow counters (only droppable topics can drop).
- `seq_commands_unrouted_total` — commands with no registered processor
  (P2-3 counter).
- `go /gc/pauses:seconds ...` and `go /sched/latencies:seconds ...` —
  runtime histograms as count/p50/p99/p99.9/max summaries, plus GC cycle and
  heap gauges.

The same overflow counters are also reported by the in-process observer
goroutine: one Warn log line per second per topic, only when a counter moved
(a healthy system logs nothing). Per-event conditions are never logged
inline — see `core/logger/doc.go`.

## 6. Verification runs

Handoff latency (acceptance: ≥ 1e8 events, isolated core, spin strategy;
p99.9 documented, max bounded):

```
$ SEQ_LATENCY_EVENTS=100000000 taskset -c 2,3 \
    go test ./core/msgbus/ -run TestPublishDispatchLatencyHarness -v -count=1
```

1-hour soak with the GC fenced (acceptance: heap high-water stable, zero GC
cycles in steady state, `/gc/pauses` count static):

```
$ GODEBUG=gctrace=1 GOGC=off GOMEMLIMIT=4GiB taskset -c 2,3 ./seq -c config/prod.yml
$ watch -n 60 curl -s http://127.0.0.1:9100/metrics   # /gc/cycles must not advance
```
