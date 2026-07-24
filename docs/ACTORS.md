# Actors

How actors work in Seq, and what they own in each domain module.

Actors are the **event-driven** unit of business logic. Engines are the **lifecycle / command / I/O** owners. Together they form the domain boundaries of the Node.

---

## 1. Actor vs Engine

| | Actor | Engine |
| --- | --- | --- |
| Role | React to events; update cache / emit commands | Construct actors, register command processors, own venue routers |
| Runs on | `Handle` on the **single dispatch thread** | Init/Start/Stop from Node; command processors also on dispatch thread |
| Talks to venues? | No (send commands instead) | Yes (via DataRouter / ExecutionRouter) |
| Config | YAML `{ type, name?, config }` under `node.engine.<domain>.actor` | Domain YAML + top-level `datarouter` / `execrouter` |

```
Adapters (WS/HTTP goroutines)
        │ Publish events / receive commands
        ▼
   MsgBus  ──Dispatch──►  Actors.Handle  ──AllocateCmd──►  Engine processors  ──► Routers ──► Venue
        ▲                      │
        └──── Publish events ──┘  (e.g. OrderNew from risk)
```

---

## 2. The `Actor` interface

`core/actor.Actor`:

```go
type Actor interface {
    Name() string
    SubscribedTypes() []event.Topic  // nil/empty = all topics
    Handle(ev msgbus.Event, bus *msgbus.MsgBus)
    OnInit(config map[string]any)
    OnStart()
    OnStop()
}
```

### Lifecycle

1. Factory constructs the actor (from YAML `type`).
2. `actor.ApplyName` applies optional YAML `name`.
3. `OnInit(config)` — decode config, resolve catalog symbols/wallets, set topics.
4. `actor.Register(bus, a)` — inject `Clock` if present; bind topics → `Handle` on msgbus.
5. Engine `Start` → `OnStart()`.
6. Dispatch loop fan-outs matching events → `Handle`.
7. Engine `Stop` → `OnStop()`.

### Bases

| Base | Package | Provides |
| --- | --- | --- |
| `ActorBase` | `core/actor` | Name, topics, `Log()`, `Clock()`, no-op lifecycle |
| `StrategyActorBase` | `strategy` | Embeds `ActorBase` + catalog/cache/msgbus; `SubmitOrder`, cancel helpers, `ReqHistoricalKline` |

Infrastructure actors (orderbook, oms, balance, risk) embed `ActorBase`. Strategies embed `StrategyActorBase`.

Go has no virtual methods: concrete types **must override `Handle`** even when embedding a base.

---

## 3. Thread model and Handle rules

The Node runs **one** dispatch consumer (`LockOSThread`):

1. Clock tick  
2. Drain **all** commands, then **one** event  
3. Fan-out the event to every actor whose `SubscribedTypes` matches  
4. Release arena memory for that event  

Rules for `Handle`:

1. **Keep it short** — you share the dispatch thread with every other actor and all command processors.
2. **No venue I/O** — do not call HTTP/WS clients; send a command and let the engine/router do I/O.
3. **Copy arena data** before return — `ReadBuffer` / zero-copy views are invalid after `Handle` returns (see [`ADDING_AN_EVENT.md`](./ADDING_AN_EVENT.md)).
4. **Commands only from the dispatch thread** — `AllocateCmd` / `Send` from `Handle` (or from an engine command processor).
5. **Log via `Log()`** so lines carry `actor=<name>`.

---

## 4. Registration (two different `Register`s)

| Mechanism | Purpose |
| --- | --- |
| `{module}.Register("type", factory)` in `init()` | YAML `type` → constructor |
| `actor.Register(bus, instance)` | Bind instance to msgbus topics |

Factories are pulled in via blank imports in `cmd/main.go`. Engines then:

```text
lookupFactory(entry.Type) → factory(...) → ApplyName → OnInit → actor.Register → append
```

Unknown `type` is logged and skipped.

---

## 5. Domain roles (DDD view)

Each engine owns one domain. Actors inside it own **reactions and cache writes** for that domain. Engines own **commands and venue I/O**.

### 5.1 Data — market data

| Piece | Kind | Owns |
| --- | --- | --- |
| `DataEngine` | Engine | `datarouter` subscriptions, DataClient connect, `ReqDepthSnapshot` / `ReqHistoricalKline` processors |
| `orderbook` | Actor | Depth snapshot/update → book state machine → **write books to cache**; gap → snapshot command |

Strategies read books from cache; they do not rebuild books themselves.

### 5.2 Execution — order lifecycle toward the venue

| Piece | Kind | Owns |
| --- | --- | --- |
| `ExecutionEngine` | Engine | Submit / cancel / cancel-all → `ExecutionRouter` (venue); paper mode refuses mutations |
| `oms` | Actor | Order lifecycle events → **write open-order cache** (`OrderNew` … filled / canceled / rejected / risk-invalid) |

OMS does not call the venue. The engine does, after risk has published `OrderNew` and sent `OrderSubmit`.

### 5.3 Ledger — balances

| Piece | Kind | Owns |
| --- | --- | --- |
| `LedgerEngine` | Engine | Subscribe private balance streams; request initial snapshots; gate system `Ready` when all wallets have snapshots |
| `balance` | Actor | One per wallet; snapshot/update (+ related) events → **write balances to cache**; signal ready to the engine |

### 5.4 Risk — pre-trade Guards

Risk actors that implement `risk.Guard` are the pre-trade gate. Guard and its state are the **same instance** (no separate checker / cache key binding).

| Piece | Kind | Owns |
| --- | --- | --- |
| `leakybucket` | Actor + Guard | Atomic admit/reject on `CommandTypeOrderRiskCheck` (sliding-window rate limit) |

Flow:

1. Strategy `SubmitOrder` → cache insert (`Initialized`) + `OrderRiskCheck` command.  
2. Engine runs Guards in YAML declaration order (short-circuit on first reject).  
3. Pass → publish `OrderNew` + send `OrderSubmit`.  
4. Fail → publish `OrderRiskInvalid` (no venue submit).

Pre-trade reject logic belongs in `Guard.Check`, not in `Handle`.

#### Risk nil topics ≠ subscribe-all

In `core/actor` / `core/msgbus`, `SubscribedTypes()` returning nil/empty means **subscribe to every topic**. Risk `Engine.Init` inverts that for this module: nil/empty topics mean **do not register** on the bus. Stateless Guards (no events to consume) must use nil topics so they are not flooded with every event. `Engine.Init` still calls `actor.InjectClock` so `Clock()` remains available without registration.

Rate-limit admit/drain uses `RiskCheck.Timestamp` (set by `SubmitOrder`), not wall-clock `time.Now()`.

### 5.5 Strategy — trading intent

| Type | Intent |
| --- | --- |
| `xarb` | Cross-exchange arb |
| `obtest` | Order-book plumbing / debug |
| `sma` | Closed-bar SMA (hist warmup + circular window) |

Strategies:

- **Read** cache (books, balances, orders) and/or consume market/order events.
- **Write** intent only through `StrategyActorBase` helpers (`SubmitOrder`, cancel, `ReqHistoricalKline`).
- **Never** call DataRouter / ExecutionRouter.

---

## 6. Ownership summary

| Concern | Writer / owner |
| --- | --- |
| Order books in cache | `orderbook` actor |
| Open orders in cache | `oms` (+ strategy insert on `SubmitOrder`); execution engine marks Cancelling |
| Balances in cache | `balance` actor |
| Rate-limit admit state | `leakybucket` Guard (in-actor; not in cache) |
| Pre-trade accept/reject | Risk **Guards** (engine path, config order) |
| Venue market data I/O | Data engine + DataRouter |
| Venue order I/O | Execution engine + ExecutionRouter |
| Trading decisions | Strategy actors |

---

## 7. Add a new actor (checklist)

1. Package under `{module}/actor/{name}/` with `init()` → `{module}.Register("yaml_type", factory)`.
2. Embed `ActorBase` (infra) or `StrategyActorBase` (strategy); set default topics in the constructor.
3. Override `Handle` + `OnInit` (mapstructure + yaml tags); use `Log()`.
4. Blank-import the package in `cmd/main.go`.
5. Add YAML under `node.engine.{module}.actor`.
6. New **commands** → register a processor on the **engine**, not on the actor.
7. New **pre-trade controls** → implement `risk.Guard` on a risk actor (`Check`); register via the risk actor factory (YAML `actor:` only).
8. Keep `Handle` non-blocking; copy arena slices out; no venue calls from strategies.

---

## 8. Related docs

- [`ARCHITECTURE.md`](./ARCHITECTURE.md) — Node topology, engines, flows  
- [`ADDING_AN_EVENT.md`](./ADDING_AN_EVENT.md) — new event/command wire types  
- `config/README.md` — YAML / secrets  
