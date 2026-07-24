# Consumer dispatch order

Dispatch order is a **correctness contract**, not an implementation detail.

## Problem

`EventBus.Dispatch()` delivers each event to consumers in registration order
(`e.consumers` slice). Registration happens in each engine's `Init` via
`actor.RegisterIn`, and engines are initialized from `node.initEngines` in a
fixed order:

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

## Registration API

There is no unphased registration. `EventBus.Register` and `actor.Register`
were removed; every consumer must state its phase.

Engines never write a phase literal — they derive it from their own type:

```go
actor.RegisterIn(e.msgBus, a, msgbus.PhaseOf(e.Type()))
```

Ordering policy lives in exactly one place: `phaseTable` in
`core/msgbus/phase.go`. Adding an engine requires adding a row; `PhaseOf`
panics on an unknown engine type rather than guessing.

## Enforcement layers

| Layer | Catches | When |
| --- | --- | --- |
| `AssertOrder` in `Node.initEngines` | Cross-phase regression (engine reorder, wrong phase) | Startup, fail-closed |
| `TestNode_ConsumerOrder` | Any change to the consumer sequence, including reordering *within* a phase | CI |
| `TestPhaseOrdering` | Reshuffling the phase constants | CI |

`AssertOrder` cannot see intra-phase reordering — two `data` actors swapping
places is invisible to it. That is what the golden test is for.

`AssertOrder` runs once at startup; the dispatch hot path is unchanged.

## Known limits

- **Intra-engine order** is not checked by `AssertOrder` (covered by the golden
  test instead).
- **Proxy, not real R/W deps.** We guard engine boundaries, not actual cache
  ownership. If a strategy actor starts writing cache, this proxy fails and a
  real ownership model is needed.
- **Commands** are out of scope: `RegisterCommand` is one processor per type.

## Related

- [`ARCHITECTURE.md`](./ARCHITECTURE.md) §4 (Node composition)
- [`ACTORS.md`](./ACTORS.md) §3 (single dispatch thread)
