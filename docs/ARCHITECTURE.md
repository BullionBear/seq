# Seq Architecture

Module-by-module review of the Lynkora trading framework (`github.com/BullionBear/seq`).

**Success condition for this document:** describe the real runtime topology, name each package’s responsibility, and document the order/event/command flows that bind engines together.

---

## 1. System overview

Seq is an **in-process, actor-oriented crypto trading runtime** written in Go. A single binary (`cmd/main.go`) boots a `node.Node` that owns:

| Concern | Owner |
| --- | --- |
| Shared pub/sub + command bus | `core/msgbus` |
| Shared read model | `core/cache` |
| Instrument / account metadata | `core/catalog` (local `instruments.json` + config-defined accounts) |
| Market data | `data` engine + `adapter` data clients |
| Order lifecycle cache | `execution` engine + OMS actor |
| Balances | `ledger` engine + balance actors |
| Pre-trade gates | `risk` engine + rules/actors |
| Trading logic | `strategy` engine + strategy actors |
| Venue I/O | `adapter/binance`, `adapter/binancefutures`, `adapter/bybit` |

There is **no PostgreSQL / GORM stack** in the current tree. Persistence today is optional plaintext msgbus logging (`.jsonl` files). Catalog metadata is local only (instruments JSON + YAML accounts).

```
                    ┌─────────────────────────────────────────┐
                    │                 Node                      │
                    │  event loop: Tick → Dispatch → Release   │
                    └─────────────────────────────────────────┘
           msgbus (events MPSC + commands SPSC)     cache (shared)
                    │                                      ▲
     ┌──────────────┼──────────────┬───────────┬───────────┼────────┐
     ▼              ▼              ▼           ▼           ▼        │
  DataEngine   ExecEngine    Ledger    RiskEngine  StrategyEngine │
  orderbook      OMS          balance     leakybucket/  xarb/obtest/ │
  + DataRouter + ExecRouter   actors      tpnl+Checker      sma      │
     │              │              │           ▲            │        │
     ▼              ▼              ▼           │            │        │
  Binance/Bybit  Binance/Bybit  private WS     │     SubmitOrder()───┘
  public WS/HTTP private WS/HTTP               │     → OrderRiskCheck
  (depth/trade/kline + hist kline REST)        └── pass → OrderSubmit
```

---

## 2. Boot and process model

### Entry (`cmd/main.go`)

1. Load YAML via `core/config.LoadConfig` (`-c` or `CONFIG`).
2. Init `core/logger` (zerolog always to stdout; optional `path` also tees to a rotating file).
3. Resolve paper/live gate via `tradingmode.Resolve(cfg.TradingMode, os.Getenv)` — default `paper`; optional override via `SEQ_TRADING_MODE`. Result is logged and passed into `Node.Init`.
4. Apply optional `runtime` fencing (`GOMAXPROCS`, `GOGC=off` + `GOMEMLIMIT`) and start the optional metrics HTTP server (`metrics` config).
5. Build `catalog.Catalog` from the local instruments file (`catalog.instruments`) and config-defined accounts (`catalog.accounts`).
6. Blank-import actor packages so `init()` registers factories (`orderbook`, `oms`, `balance`, `ratelimiter` package → type `leakybucket`, `tpnl`, `obtest`, `sma`, `xarb`).
7. `node.NewNode` → wire msgbus, cache, five engines, execution router.
8. Optional `msgbus.MsgLogger` for plaintext JSONL event/command audit logs.
9. `Init` → `Start` → `Run` (blocks until SIGINT/SIGTERM).

### Node event loop (`node/node.go`)

Single consumer loop, pinned with `runtime.LockOSThread()`:

1. Drive `clock.Clock` via `msgBus.GetTicker().Tick(nowNs)`.
2. `msgBus.Dispatch()` — **drain all commands first**, then one event.
3. On work: `Release` / `ReleaseArenas` for arena reclaim; else idle via configurable strategy (`gosched` default, or bounded `spin`).

An overflow observer logs moved drop/wait counters at ~1 Hz when non-zero.

Shutdown order is intentional for safety:

1. Stop strategies (emit cancels).
2. Disconnect data clients.
3. Drain msgbus (~3s / idle rounds).
4. Stop risk / execution / ledger actors.
5. Disconnect execution clients.

---

## 3. Core platform (`core/`)

### 3.1 `core/msgbus` — spine of the system

Dual-channel bus:

| Channel | Topology | Producer | Consumer |
| --- | --- | --- | --- |
| **Events** | MPSC ring + byte arena | Adapters / engines (any goroutine) | Registered actors (dispatch thread) |
| **Commands** | SPSC ring + byte arena | Dispatch thread only (during `Handle`) | Exactly one processor per `CommandType` |

Design notes:

- Events are **topic-filtered fan-out** to actors.
- Commands are **point-to-point** and always higher priority than events.
- Payloads live in arenas; refs carry `(topic|commandType, index, length)`.
- Optional `MsgLogger` writes decoded payloads as JSONL for auditability.
- **Overflow policy (events):** droppable topics (depth, tick, kline, timer) may spin then drop with counters; critical topics (orders, balances, engine state, hist-kline responses, …) wait and **fatal** after a deadline. Command ring/arena overflow is also fatal — never silently dropped.

### 3.2 `core/actor`

`Actor` is the unit of business logic: `Name`, `SubscribedTypes`, `Handle`, `OnInit`/`OnStart`/`OnStop`.

`actor.Register` binds the actor to msgbus and injects `clock.Clock` when present.

Engines construct actors from YAML via per-package factory registries (`Register("type", factory)` in `init()`).

Actor lifecycle, domain ownership, and conventions: [`ACTORS.md`](./ACTORS.md).

### 3.3 `core/engine`

`Engine` interface + `EngineBase` for lifecycle and state notifications (`Ready` / `Stop` / `Finished` / `Abnormal`) published through `msgbus.StateNotifier`. Domain engines embed this.

### 3.4 `core/model`

Binary-encoded domain types for the in-memory bus:

- `model/common` — orders, depth levels, balances, enums (`Side`, `OrderType`, `Interval`, exchanges, wallet types).
- `model/event` — market (depth, trade, **kline**, **hist-kline response**), order, balance, timer, and engine-state events + `Topic` enum.
- `model/command` — `OrderRiskCheck`, `OrderSubmit`, `OrderCancel`, `CancelAll`, `ReqDepthSnapshot`, `ReqHistoricalKline`, `QryBalanceSnapshot`.

Fixed-size codecs use shared `core/model/codec` (`Encode` / `Decode` with bounds checks + POD/layout guards). Variable-size types (depth snapshots/updates, hist-kline responses) keep specialized codecs/views.

To add a new event type end-to-end, see [`ADDING_AN_EVENT.md`](./ADDING_AN_EVENT.md).

### 3.5 `core/cache`

In-memory read model shared across engines/strategies (no engine package imports):

- Order books (btree price levels per symbol).
- Open orders (`OrderCache`).
- Balances per account/token.
- Risk metadata (e.g. rate-limit next-accept time).
- TPNL window state.

Writers: data/execution/ledger/risk actors. Readers: strategies and risk rules.

### 3.6 `core/catalog`

Local instrument/account registry: symbols load from a JSON file (`catalog.instruments`), accounts/wallets/API keys from the YAML config (`catalog.accounts`, secrets via `${ENV_VAR}` expansion). Resolves universal tickers (`BINANCE_SPOT_UNIUSDT`), accounts, wallets, API key names used by adapters.

### 3.7 Supporting core packages

| Package | Role |
| --- | --- |
| `core/mem` | SPSC/MPSC ring buffers, consumer-bounded circular byte arenas |
| `core/clock` | Timer ticks → `TopicEventTimer` on the bus |
| `core/logger` | Singleton zerolog wrapper (stdout always; optional file via `path`) |
| `core/config` | Top-level `AppConfig` YAML unmarshal |
| `core/env` | Build-time version/commit/time (ldflags) |
| `core/tradingmode` | Paper/live gate resolution |
| `core/telemetry` | Runtime fencing (`gc_off`, mem limit, `GOMAXPROCS`) + `/metrics` + `POST /gc` |

---

## 4. Node composition (`node/`)

`Node` owns engines and routers. `Init` responsibilities:

1. Capture `node.dispatch` idle strategy / spin budget; pin dispatch OS thread at `Run`.
2. Attach `StateNotifier` to ledger.
3. From `execrouter` YAML: resolve catalog accounts/wallets, create Binance/Bybit execution clients, register on `ExecutionRouter` (live mutations gated by trading mode).
4. Init engines with `node.engine.*` YAML + `datarouter` subscriptions for data.

Start order: data → execution connect/start → ledger → risk → strategy.

---

## 5. Domain engines

### 5.1 Data (`data/`)

- Parses `datarouter` entries → symbol IDs + depth / trade / **kline** options.
- Lazily builds venue `DataClient`s via `adapter.DataRouter` (Binance/Bybit spot).
- Actors: `orderbook` maintains cache books from depth snapshot/update events; can request snapshots via `CommandTypeReqDepthSnapshot`.
- Handles `CommandTypeReqHistoricalKline` → venue REST → `TopicEventRespHistoricalKline`.
- `Start` connects WebSockets after subscriptions are prepared in `Init`.

### 5.2 Execution (`execution/`)

- Command processor for submit / cancel / cancel-all → `ExecutionRouter`.
- Cancel path does **optimistic** cache update to `Cancelling` before venue call.
- Actor: `oms` consumes order lifecycle events and keeps `cache` order state coherent (`OrderNew` → accepted / fills / cancel / reject / risk-invalid).

### 5.3 Ledger (`ledger/`)

- Subscribes balances on execution clients; requests initial snapshots on start.
- Actor: `balance` writes snapshot/update events into cache; signals engine readiness when all balance actors have snapshots (ledger gates system `Ready`).

### 5.4 Risk (`risk/`)

Two layers:

1. **Actors** (YAML type `leakybucket` rate limiter, `tpnl`) update cache risk/TPNL state from events/timers.
2. **Checker** — ordered rule list (`ratelimit`, `tpnl` stop-loss) evaluated on `CommandTypeOrderRiskCheck`.

On pass: publish `OrderNew` event + send `OrderSubmit` command.  
On fail: publish `OrderRiskInvalid` (no submit).

### 5.5 Strategy (`strategy/`)

- Factory-built actors from YAML (`xarb`, `obtest`, `sma`).
- `StrategyActorBase.SubmitOrder` inserts an initialized order into cache, then sends `OrderRiskCheck` (never submits directly to the venue).
- Helpers: cancel commands; `ReqHistoricalKline` for bar warmup.

Built-in strategies:

| Type | Intent |
| --- | --- |
| `xarb` | Cross-exchange arb (quote one venue, hedge the other) |
| `obtest` | Order-book / plumbing test strategy |
| `sma` | Closed-bar SMA over a circular window; warms up via historical klines |

---

## 6. Adapters (`adapter/`)

### Interfaces

- `DataClient` — public depth / trade / **kline** streams + REST depth snapshot + **historical kline**.
- `ExecutionClient` — connect, granular private subscriptions (orders/fills/balances), submit/cancel, balance snapshot.

### Routers

- `DataRouter` — exchange+product keyed factories; translates YAML depth / trade / kline options to venue params.
- `ExecutionRouter` — account-ID keyed client map; fan-in API for engines; refuses venue submit/cancel in paper mode.

### Venues

| Package | Coverage (current) |
| --- | --- |
| `adapter/binance` | Spot data + spot execution (WS + fasthttp); live + hist kline |
| `adapter/binancefutures` | USD-M perpetual data + execution (`fstream` / `fapi` / `ws-fapi`); live + hist kline |
| `adapter/bybit` | Spot data + execution (WS + HTTP); live + hist kline |

Adapters publish **normalized** events onto msgbus; they do not call strategy code. Live kline streams include both open (`Closed=false`) and closed (`Closed=true`) bars.

There is no rich unified venue API: each stream parser maps venue JSON onto shared events. Market-data coverage vs exchange docs: [`adapter/binance/BINANCE.md`](../adapter/binance/BINANCE.md), [`adapter/binancefutures/BINANCE_FUTURES.md`](../adapter/binancefutures/BINANCE_FUTURES.md), [`adapter/bybit/BYBIT.md`](../adapter/bybit/BYBIT.md).

---

## 7. Configuration model

Top-level YAML (`core/config.AppConfig`):

```yaml
trading_mode: paper|live   # optional; default paper; SEQ_TRADING_MODE overrides
logger:
  level: debug
  path: logs/seq.log       # optional; stdout always on
  max_byte_size: 10485760
  max_backup_files: 5
msgbus: { msglog: { enabled, dir } }
catalog: { instruments, accounts: [ { name, exchange, api_keys: [...], wallets: [...] } ] }
execrouter: [ { account, wallet, api } ]
datarouter: [ { symbol, depth?, trade?, kline?, endpoint? } ]
runtime:                   # optional
  gomaxprocs: 4
  gc_off: true
  mem_limit_bytes: 4294967296
metrics:                   # optional
  enabled: true
  addr: 127.0.0.1:9100
node:
  dispatch:
    idle_strategy: gosched # or spin
    spin_budget: 4096
  engine:
    data: { actor: [...] }
    execution: { actor: [...] }
    ledger: { actor: [...] }
    risk: { actor: [...], checker: [...] }
    strategy: { actor: [...] }
```

Actor entries are uniformly `{ type, name?, config: map }`. Sample scenarios live under `config/` (`xarb.yml`, `obtest.yml`, `sma.yml`, `test.yml`) with `${ENV_VAR}` placeholders for API keys/secrets; see `config/README.md`.

**Security note:** sample YAML no longer embeds credentials. Credentials that were previously committed remain in git history until rotated at the venue and (if needed) history is rewritten.

---

## 8. Critical runtime flows

### 8.1 Market data → strategy

1. Data WS callback encodes depth / trade / kline → `Publish` event.
2. Dispatch delivers to subscribed actors (`orderbook`, strategies).
3. Orderbook updates `cache`; strategies read best bid/ask / depth from cache and/or consume kline events.

### 8.2 Historical kline warmup

1. Strategy (e.g. `sma`) calls `ReqHistoricalKline` on first live bar.
2. Data engine → venue REST → `TopicEventRespHistoricalKline` (oldest → newest).
3. Strategy copies closes into its own buffer (arena view is only valid during `Handle`).

### 8.3 Order intent → venue

1. Strategy `SubmitOrder` → cache insert (`Initialized`) + `OrderRiskCheck` command.
2. Risk `Checker` runs rules.
3. Pass → `OrderNew` event + `OrderSubmit` command; fail → `OrderRiskInvalid`.
4. OMS handles `OrderNew` (cache). Execution engine sends submit to venue via router (refused in paper mode).
5. Venue private stream → accepted / fill / cancel / reject events → OMS updates cache; strategies react.

### 8.4 Cancel / shutdown

Strategy or stop path → `OrderCancel` / cancel-all → execution engine marks cancelling → venue cancel → confirmation events → OMS.

---

## 9. Observability and durability

| Mechanism | What it covers |
| --- | --- |
| Zerolog structured logs | Ops / debug (stdout; optional file) |
| Msgbus msglog (`.jsonl`) | Plaintext event+command audit trail |
| Engine state events | Ready / stop / abnormal fan-out |
| `core/telemetry` metrics | `GET /metrics` (drop/wait/unrouted counters + runtime GC/sched histograms); `POST /gc` |
| Msgbus overflow observer | ~1 Hz Warn when drop/wait counters move |

Still open relative to a full trading desk: reject/slippage dashboards, kill-switch service, deterministic backtest harness. Paper-vs-live execution gate is implemented (`trading_mode`; see `core/tradingmode`).

---

## 10. Build and quality

- Go **1.26.0**, module `github.com/BullionBear/seq`.
- `Makefile`: local/linux builds with version ldflags, `go test -race`, coverage, benchmarks, escape analysis.
- CI: `.github/workflows/go.yml`.
- `docker-compose.yml` still defines a legacy Postgres service; `README.md` correctly disclaims it as not part of this runtime.

---

## 11. Package map (source of truth)

```
seq/
├── cmd/                 # seq binary
├── config/              # YAML scenarios
├── docs/                # architecture (this file)
├── node/                # composition root + event loop
├── core/
│   ├── actor|engine|msgbus|mem|clock|cache|catalog|config|logger|env
│   ├── tradingmode|telemetry|model (+ model/codec)
├── adapter/             # DataRouter, ExecutionRouter, binance/, bybit/
├── data/                # market-data engine + orderbook actor
├── execution/           # order engine + oms actor
├── ledger/           # balance engine + balance actor
├── risk/                # risk engine, checker, rules, ratelimiter/tpnl actors
└── strategy/            # strategy engine + xarb/obtest/sma (+ StrategyActorBase)
```

---

## 12. Architectural assessment

**Strengths**

- Clear separation: adapters normalize I/O; engines own commands; actors own state/reactions; strategies only talk to bus/cache.
- Command-before-event dispatch and arena-backed payloads suit low-latency in-process trading.
- Risk gate sits on the mandatory path between strategy intent and venue submit.
- Config-driven actor factories keep the binary “all-in-one” while allowing scenario YAML composition.
- Optional plaintext msglog (JSONL) supports post-trade audit.
- Overflow policy separates droppable market data from critical order/balance events.

**Risks / follow-ups**

1. **Secrets in sample YAML** — scrubbed from working tree (`${ENV_VAR}` placeholders + gitignored `*.local.yml`); **rotate** any previously exposed credentials at the venue (history may still contain them).
2. **Live trading safety** — addressed by `core/tradingmode` + execution-router gate: default `paper`; `live` requires `trading_mode=live` (or `SEQ_TRADING_MODE=live`), logged at boot.
3. **Single-process blast radius** — one Node hosts data, risk, execution, and strategy; process crash stops everything (acceptable for early firm stage; revisit isolation later).
4. **Command/critical overflow** — full command ring or critical event backlog fatals the process; needs ops alerting on the overflow counters before that happens.
5. **Research/backtest** — plaintext msglog JSONL is available for audit/research; full deterministic replay/backtest stack is not yet a first-class module.

---

## 13. Suggested next engineering work (out of scope for this review)

1. Add paper execution adapter (venue order mutations are already gated; paper fill simulation remains).
2. Trading dashboards: reject rates, slippage, inventory, PnL (runtime `/metrics` already covers bus overflow + GC/sched).
3. Expand venue/product coverage beyond spot Binance/Bybit as needed.
4. Deterministic backtest / replay harness over msglog or recorded streams.
5. Kill-switch / external risk control service.
