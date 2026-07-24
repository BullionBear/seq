# Consumer dispatch order

Dispatch order is a **correctness contract**, not an implementation detail.

## Problem

`EventBus.Dispatch()` delivers each event to consumers in registration order
(`e.consumers` slice). Registration happens in each engine's `Init` via
`actor.RegisterIn`, and engines are initialized from `node.Init` in a fixed
order:

```text
data → execution → ledger → risk → strategy
```

Correctness depends on one invariant: **for the same event, actors that write
cache must run before actors that read that cache.**

Example: `orderbook` handles `DepthUpdate` and updates the book; `xarb` handles
the same `DepthUpdate` and calls `GetBestBid` / `GetBestAsk`. If strategy ran
first, it would price off the **previous** book.

Historically this was guaranteed only by the line order of five `Init` calls in
`node.go` — no type, no assertion, no test. Reordering those lines compiled,
started, and silently used stale books (intermittently: only when that tick
moved best bid/ask).

## Invariant

Formal:

> For any topic `T`, if consumer A writes cache region `R` while handling `T`
> and consumer B reads `R` while handling `T`, then A must precede B.

In practice this aligns with engine boundaries, so we enforce a weaker proxy:

> Phases of consumers in `e.consumers` must be **non-decreasing**.

| Phase | Constant | Engine | Role |
| --- | --- | --- | --- |
| 0 | `PhaseIngest` | `data` | Write orderbook / kline |
| 1 | `PhaseOrder` | `execution` | Write order cache |
| 2 | `PhaseAccount` | `ledger` | Write balance / position |
| 3 | `PhaseControl` | `risk` | (guards with nil topics do not register) |
| 4 | `PhaseDecide` | `strategy` | Read-only |

This is the order the system already runs. Enforcement does not change it.

## Enforcement

1. Engines register with `actor.RegisterIn(bus, a, phase)`.
2. After all engines init, `Node.Init` calls `msgBus.AssertOrder()` and
   **fatals** on violation.
3. `ConsumerNames()` exposes the dispatch sequence for inspection / tests.

`AssertOrder` runs once at startup; the dispatch hot path is unchanged.

## Known limits

- **Intra-engine order** is not checked. Two actors in the same phase can still
  be wrong relative to each other; the phase assert only covers cross-engine
  order.
- **Proxy, not real R/W deps.** We guard engine boundaries, not actual cache
  ownership. If a strategy actor starts writing cache, this proxy fails and a
  real ownership model is needed.
- **Commands** are out of scope: `RegisterCommand` is one processor per type.

## Related

- [`ARCHITECTURE.md`](./ARCHITECTURE.md) §4 (Node composition)
- [`ACTORS.md`](./ACTORS.md) §3 (single dispatch thread)
