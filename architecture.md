# Seq Architecture

Module-by-module review of the Lynkora trading framework (`github.com/BullionBear/seq`).

**Success condition for this document:** describe the real runtime topology (not the outdated README), name each package’s responsibility, and document the order/event/command flows that bind engines together.

---

## 1. System overview

Seq is an **in-process, actor-oriented crypto trading runtime** written in Go. A single binary (`cmd/main.go`) boots a `node.Node` that owns:

| Concern | Owner |
| --- | --- |
| Shared pub/sub + command bus | `core/msgbus` |
| Shared read model | `core/cache` |
| Instrument / account metadata | `core/catalog` (+ remote cpanel API) |
| Market data | `data` engine + `adapter` data clients |
| Order lifecycle cache | `execution` engine + OMS actor |
| Balances | `portfolio` engine + balance actors |
| Pre-trade gates | `risk` engine + rules/actors |
| Trading logic | `strategy` engine + strategy actors |
| Venue I/O | `adapter/binance`, `adapter/bybit` |

There is **no PostgreSQL / GORM stack** in the current tree. Persistence today is optional binary msgbus logging (`.dat` files) plus remote catalog. The root `README.md` still describes an older PMS/EMS/SMS layout and is **out of date**.

```
                    ┌─────────────────────────────────────────┐
                    │                 Node                      │
                    │  event loop: Tick → Dispatch → Release   │
                    └─────────────────────────────────────────┘
           msgbus (events MPSC + commands SPSC)     cache (shared)
                    │                                      ▲
     ┌──────────────┼──────────────┬───────────┬───────────┼────────┐
     ▼              ▼              ▼           ▼           ▼        │
  DataEngine   ExecEngine    Portfolio    RiskEngine  StrategyEngine │
  orderbook      OMS          balance     ratelimit/     xarb/obtest │
  + DataRouter + ExecRouter   actors      tpnl+Checker      │        │
     │              │              │           ▲            │        │
     ▼              ▼              ▼           │            │        │
  Binance/Bybit  Binance/Bybit  private WS     │     SubmitOrder()───┘
  public WS/HTTP private WS/HTTP               │     → OrderRiskCheck
                                               └── pass → OrderSubmit
```

---

## 2. Boot and process model

### Entry (`cmd/main.go`)

1. Load YAML via `core/config.LoadConfig` (`-c` or `CONFIG`).
2. Init `core/logger` (zerolog + optional lumberjack rotation).
3. Build `catalog.Catalog` from cpanel (`base_url` + `api_token`); load symbols and accounts.
4. Blank-import actor packages so `init()` registers factories (`orderbook`, `oms`, `balance`, `ratelimiter`, `tpnl`, `obtest`, `xarb`).
5. `node.NewNode` → wire msgbus, cache, five engines, execution router.
6. Optional `msgbus.MsgLogger` for binary event/command audit logs.
7. `Init` → `Start` → `Run` (blocks until SIGINT/SIGTERM).

### Node event loop (`node/node.go`)

Single consumer loop:

1. Drive `clock.Clock` via `msgBus.GetTicker().Tick(nowNs)`.
2. `msgBus.Dispatch()` — **drain all commands first**, then one event.
3. On work: `Release` / `ReleaseArenas` for arena reclaim; else `runtime.Gosched()`.

Shutdown order is intentional for safety:

1. Stop strategies (emit cancels).
2. Disconnect data clients.
3. Drain msgbus (~3s / idle rounds).
4. Stop risk / execution / portfolio actors.
5. Disconnect execution clients.

### Parser tool (`cmd/parser`)

Offline helper to deserialize `event_*.dat` / command logs to JSONL (`make parse-event`). Supports research/audit replay of the msglog.

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
- Optional `MsgLogger` persists binary payloads for auditability.

### 3.2 `core/actor`

`Actor` is the unit of business logic: `Name`, `SubscribedTypes`, `Handle`, `OnInit`/`OnStart`/`OnStop`.

`actor.Register` binds the actor to msgbus and injects `clock.Clock` when present.

Engines construct actors from YAML via per-package factory registries (`Register("type", factory)` in `init()`).

### 3.3 `core/engine`

`Engine` interface + `EngineBase` for lifecycle and state notifications (`Ready` / `Stop` / `Finished` / `Abnormal`) published through `msgbus.StateNotifier`. Domain engines embed this.

### 3.4 `core/model`

Binary-encoded domain types:

- `model/common` — orders, depth levels, balances, enums (`Side`, `OrderType`, exchanges, wallet types).
- `model/event` — market, order, balance, timer, and engine-state events + `Topic` enum.
- `model/command` — `OrderRiskCheck`, `OrderSubmit`, `OrderCancel`, `CancelAll`, `ReqDepthSnapshot`, `QryBalanceSnapshot`.

Codecs are hand-written (`*_codec.go`) for low-allocation encode/decode into arena buffers.

### 3.5 `core/cache`

In-memory read model shared across engines/strategies (no engine package imports):

- Order books (btree price levels per symbol).
- Open orders (`OrderCache`).
- Balances per account/token.
- Risk metadata (e.g. rate-limit next-accept time).
- TPNL window state.

Writers: data/execution/portfolio/risk actors. Readers: strategies and risk rules.

### 3.6 `core/catalog`

Remote instrument/account registry via `core/catalog/cpanel` HTTP client. Resolves universal tickers (`BINANCE_SPOT_UNIUSDT`), accounts, wallets, API key names used by adapters.

### 3.7 Supporting core packages

| Package | Role |
| --- | --- |
| `core/mem` | SPSC/MPSC ring buffers, simple byte arenas |
| `core/clock` | Timer ticks → `TopicEventTimer` on the bus |
| `core/logger` | Singleton zerolog wrapper |
| `core/config` | Top-level `AppConfig` YAML unmarshal |
| `core/env` | Build-time version/commit/time (ldflags) |

---

## 4. Node composition (`node/`)

`Node` owns engines and routers. `Init` responsibilities:

1. Attach `StateNotifier` to portfolio.
2. From `execrouter` YAML: resolve catalog accounts/wallets, create Binance/Bybit execution clients, register on `ExecutionRouter`.
3. Init engines with `node.engine.*` YAML + `datarouter` subscriptions for data.

Start order: data → execution connect/start → portfolio → risk → strategy.

---

## 5. Domain engines

### 5.1 Data (`data/`)

- Parses `datarouter` entries → symbol IDs + depth/trade options.
- Lazily builds venue `DataClient`s via `adapter.DataRouter` (Binance/Bybit spot).
- Actors: `orderbook` maintains cache books from depth snapshot/update events; can request snapshots via `CommandTypeReqDepthSnapshot`.
- `Start` connects WebSockets after subscriptions are prepared in `Init`.

### 5.2 Execution (`execution/`)

- Command processor for submit / cancel / cancel-all → `ExecutionRouter`.
- Cancel path does **optimistic** cache update to `Cancelling` before venue call.
- Actor: `oms` consumes order lifecycle events and keeps `cache` order state coherent (`OrderNew` → accepted / fills / cancel / reject / risk-invalid).

### 5.3 Portfolio (`portfolio/`)

- Subscribes balances on execution clients; requests initial snapshots on start.
- Actor: `balance` writes snapshot/update events into cache; signals engine readiness when all balance actors have snapshots (portfolio gates system `Ready`).

### 5.4 Risk (`risk/`)

Two layers:

1. **Actors** (e.g. `leakybucket` rate limiter, `tpnl`) update cache risk/TPNL state from events/timers.
2. **Checker** — ordered rule list (`ratelimit`, `tpnl` stop-loss) evaluated on `CommandTypeOrderRiskCheck`.

On pass: publish `OrderNew` event + send `OrderSubmit` command.  
On fail: publish `OrderRiskInvalid` (no submit).

### 5.5 Strategy (`strategy/`)

- Factory-built actors from YAML (`xarb`, `obtest`).
- `StrategyActorBase.SubmitOrder` inserts an initialized order into cache, then sends `OrderRiskCheck` (never submits directly to the venue).
- Cancel helpers emit `OrderCancel` / related commands on the bus.

Built-in strategies:

| Type | Intent |
| --- | --- |
| `xarb` | Cross-exchange arb (quote one venue, hedge the other) |
| `obtest` | Order-book / plumbing test strategy |

---

## 6. Adapters (`adapter/`)

### Interfaces

- `DataClient` — public depth/trade streams + REST depth snapshot.
- `ExecutionClient` — connect, granular private subscriptions (orders/fills/balances), submit/cancel, balance snapshot.

### Routers

- `DataRouter` — exchange+product keyed factories; translates YAML depth/trade options to venue params.
- `ExecutionRouter` — account-ID keyed client map; fan-in API for engines.

### Venues

| Package | Coverage (current) |
| --- | --- |
| `adapter/binance` | Spot data + spot execution (WS + fasthttp) |
| `adapter/bybit` | Spot data + execution (WS + HTTP) |

Adapters publish **normalized** events onto msgbus; they do not call strategy code.

---

## 7. Configuration model

Top-level YAML (`core/config.AppConfig`):

```yaml
logger: { ... }
msgbus: { msglog: { enabled, dir } }
catalog: { base_url, api_token }
execrouter: [ { account, wallet, api } ]
datarouter: [ { symbol, depth?, trade?, endpoint? } ]
node:
  engine:
    data: { actor: [...] }
    execution: { actor: [...] }
    portfolio: { actor: [...] }
    risk: { actor: [...], checker: [...] }
    strategy: { actor: [...] }
```

Actor entries are uniformly `{ type, name?, config: map }`. Sample scenarios live under `config/` (`xarb.yml`, `obtest.yml`, `test.yml`) with `${CATALOG_API_TOKEN}` placeholders; see `config/README.md`.

**Security note:** sample YAML no longer embeds catalog API tokens. Credentials that were previously committed remain in git history until rotated at the provider and (if needed) history is rewritten.

---

## 8. Critical runtime flows

### 8.1 Market data → strategy

1. Data WS callback encodes depth/tick → `Publish` event.
2. Dispatch delivers to subscribed actors (`orderbook`, strategies).
3. Orderbook updates `cache`; strategies read best bid/ask / depth from cache.

### 8.2 Order intent → venue

1. Strategy `SubmitOrder` → cache insert (`Initialized`) + `OrderRiskCheck` command.
2. Risk `Checker` runs rules.
3. Pass → `OrderNew` event + `OrderSubmit` command; fail → `OrderRiskInvalid`.
4. OMS handles `OrderNew` (cache). Execution engine sends submit to venue via router.
5. Venue private stream → accepted / fill / cancel / reject events → OMS updates cache; strategies react.

### 8.3 Cancel / shutdown

Strategy or stop path → `OrderCancel` / cancel-all → execution engine marks cancelling → venue cancel → confirmation events → OMS.

---

## 9. Observability and durability

| Mechanism | What it covers |
| --- | --- |
| Zerolog structured logs | Ops / debug |
| Msgbus msglog (`.dat`) | Binary event+command audit trail |
| `cmd/parser` | Offline decode to JSONL |
| Engine state events | Ready / stop / abnormal fan-out |

Gaps relative to a production trading desk (not present as first-class modules today): metrics/latency histograms, reject/slippage dashboards, paper-vs-live execution gate, kill-switch service, deterministic backtest harness.

---

## 10. Build and quality

- Go **1.25.1**, module `github.com/BullionBear/seq`.
- `Makefile`: local/linux builds with version ldflags, `go test -race`, coverage, benchmarks, escape analysis, parser targets.
- CI: `.github/workflows/go.yml`.
- `docker-compose.yml` present (legacy README still assumes Postgres; verify before relying on it for this architecture).

---

## 11. Package map (source of truth)

```
seq/
├── cmd/                 # seq binary + event log parser
├── config/              # YAML scenarios
├── node/                # composition root + event loop
├── core/
│   ├── actor|engine|msgbus|mem|clock|cache|catalog|config|logger|env|model
├── adapter/             # DataRouter, ExecutionRouter, binance/, bybit/
├── data/                # market-data engine + orderbook actor
├── execution/           # order engine + oms actor
├── portfolio/           # balance engine + balance actor
├── risk/                # risk engine, checker, rules, ratelimiter/tpnl actors
└── strategy/            # strategy engine + xarb/obtest (+ StrategyActorBase)
```

---

## 12. Architectural assessment

**Strengths**

- Clear separation: adapters normalize I/O; engines own commands; actors own state/reactions; strategies only talk to bus/cache.
- Command-before-event dispatch and arena-backed payloads suit low-latency in-process trading.
- Risk gate sits on the mandatory path between strategy intent and venue submit.
- Config-driven actor factories keep the binary “all-in-one” while allowing scenario YAML composition.
- Optional binary msglog supports post-trade audit.

**Risks / follow-ups**

1. **README drift** — document still describes PMS/EMS/SMS + Postgres; replace or rewrite against this file.
2. **Secrets in sample YAML** — scrubbed from working tree (`${CATALOG_API_TOKEN}` + gitignored `*.local.yml`); **rotate** any previously exposed tokens at the provider (history may still contain them).
3. **Live trading safety** — no explicit paper/live mode or CEO-gated live switch in code; operational discipline is config/ops only.
4. **Single-process blast radius** — one Node hosts data, risk, execution, and strategy; process crash stops everything (acceptable for early firm stage; revisit isolation later).
5. **Command drop on full ring** — msgbus logs and drops when command SPSC is full; needs monitoring/alerting.
6. **Research/backtest** — msglog parser exists; full deterministic replay/backtest stack is not yet a first-class module.

---

## 13. Suggested next engineering work (out of scope for this review)

1. Align `README.md` with this architecture.
2. Add paper execution adapter + explicit live gate (CEO/board approval required for live enablement).
3. Metrics: queue depth, dispatch lag, reject rates, slippage, inventory, PnL.
4. Harden secret handling (env/secret store; no tokens in git).
5. Expand venue/product coverage beyond spot Binance/Bybit as needed.
```
