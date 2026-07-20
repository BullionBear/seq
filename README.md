# Seq

In-process, actor-oriented crypto trading runtime for Lynkora (`github.com/BullionBear/seq`).

A single Go binary boots a `node.Node` that owns market data, execution, ledger, risk, and strategy engines over a shared msgbus and cache. Venue I/O is normalized through Binance/Bybit adapters.

For the module-by-module source of truth (package responsibilities, boot order, order/event/command flows), see [`architecture.md`](./architecture.md).

## Overview

| Concern | Owner |
| --- | --- |
| Shared pub/sub + command bus | `core/msgbus` |
| Shared read model | `core/cache` |
| Instrument / account metadata | `core/catalog` (local `instruments.json` + config-defined accounts) |
| Market data | `data` engine + `adapter` data clients |
| Order lifecycle | `execution` engine + OMS actor |
| Balances | `ledger` engine + balance actors |
| Pre-trade gates | `risk` engine + rules/actors |
| Trading logic | `strategy` engine + strategy actors |
| Venue I/O | `adapter/binance`, `adapter/bybit` |

There is **no PostgreSQL / GORM stack** in the current tree. Persistence today is optional plaintext msgbus logging (`.jsonl` files) plus the remote catalog. The legacy `docker-compose.yml` Postgres service is not part of this runtime.

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
  orderbook      OMS          balance     ratelimit/     xarb/obtest │
  + DataRouter + ExecRouter   actors      tpnl+Checker      │        │
     │              │              │           ▲            │        │
     ▼              ▼              ▼           │            │        │
  Binance/Bybit  Binance/Bybit  private WS     │     SubmitOrder()───┘
  public WS/HTTP private WS/HTTP               │     → OrderRiskCheck
                                               └── pass → OrderSubmit
```

## Features

- **In-process Node** — single consumer loop: clock tick → command-before-event dispatch → arena release
- **Dual-channel msgbus** — MPSC events (topic fan-out) and SPSC commands (point-to-point, higher priority)
- **Shared cache** — order books, open orders, balances, and risk metadata as the cross-engine read model
- **Mandatory risk gate** — strategies call `SubmitOrder` → `OrderRiskCheck`; venue submit only on pass
- **Venue adapters** — Binance/Bybit spot data + execution (WS + HTTP); adapters publish normalized events only
- **Config-driven actors** — YAML factories for orderbook, OMS, balance, ratelimiter, tpnl, `xarb`, `obtest`
- **Optional msglog** — plaintext JSONL event/command audit trail written at dispatch
- **Structured logging** — zerolog with optional lumberjack rotation

## Getting started

### Prerequisites

- Go **1.25.1** or later
- Make (for build automation)
- Venue API credentials for live scenarios (not required to build or unit-test)

### Install and build

```bash
git clone https://github.com/BullionBear/seq.git
cd seq

make deps
make build-local          # → bin/seq
# or
make build                # → bin/seq-linux-amd64
```

### Configure

Instruments (symbols) load from a local JSON file (`catalog.instruments`, e.g. `config/instruments.json`). Accounts, wallets, and API keys are defined under `catalog.accounts`; key/secret values use `${ENV_VAR}` placeholders (no live secrets in git). See [`config/README.md`](config/README.md).

```bash
export BYBIT_HEPHE_API_KEY='...' BYBIT_HEPHE_API_SECRET='...'
# or: cp config/obtest.yml config/obtest.local.yml  # gitignored; edit secrets there
```

Trading mode defaults to **paper**. Live venue order submit/cancel requires `trading_mode: live` (or `SEQ_TRADING_MODE=live`) — do not enable live casually.

### Run

```bash
./bin/seq -c config/myconfig.yml
# or
CONFIG=config/myconfig.yml ./bin/seq
```

`make run` builds `bin/seq` but does not pass a config path; always supply `-c` or `CONFIG`.

When `msgbus.msglog.enabled` is true, the node writes date-stamped
`event_YYYY-MM-DD.jsonl` and `command_YYYY-MM-DD.jsonl` files under
`msgbus.msglog.dir` (one JSON object per line).

## Configuration

Top-level YAML (`core/config.AppConfig`). Actor entries are uniformly `{ type, name?, config: map }`.

```yaml
# Defaults to paper when omitted.
trading_mode: paper

logger:
  level: debug              # trace, debug, info, warn, error, fatal, panic
  output: stdout            # "stdout" or "file"
  path: logs/seq.log        # required when output is "file"
  max_byte_size: 10485760
  max_backup_files: 5

msgbus:
  msglog:
    enabled: true
    dir: ./logs/

catalog:
  instruments: ./instruments.json   # relative to this config file's directory
  accounts:
    - name: <account_name>
      exchange: Binance             # Binance | Bybit
      api_keys:
        - name: <api_key_name>
          type: HMAC                # HMAC | RSA | ED25519
          key: ${MY_API_KEY}        # never commit real keys
          secret: ${MY_API_SECRET}
      wallets:
        - name: <wallet_name>
          type: spot                # spot | umargin | cmargin | leverage | unified

execrouter:
  - account: <account_name>
    wallet: <wallet_name>
    api: <api_key_name>

datarouter:
  - symbol: BINANCE_SPOT_BTCUSDT
    depth:
      levels: 50
    # trade: ...
    # endpoint: ...

node:
  engine:
    data:
      actor:
        - type: orderbook
          config:
            symbol: BINANCE_SPOT_BTCUSDT
    execution:
      actor:
        - type: oms
          config: {}
    ledger:
      actor:
        - type: balance
          config: {}
    risk:
      actor:
        - type: ratelimiter
          config: {}
        - type: tpnl
          config: {}
      checker:
        - type: ratelimit
        - type: tpnl
    strategy:
      actor:
        - type: obtest   # or xarb
          config:
            symbol_universal_ticker: BINANCE_SPOT_BTCUSDT
```

Sample scenarios: `config/obtest.yml`, `config/xarb.yml`, `config/test.yml` (placeholders only; see `config/README.md`).

**Security:** sample configs no longer embed catalog tokens. Tokens that were previously committed must still be **rotated at the provider** — scrubbing the tree does not revoke exposed credentials.

## Critical runtime flows

### Market data → strategy

1. Data WS callback encodes depth/tick → publish event on msgbus
2. Dispatch delivers to subscribed actors (`orderbook`, strategies)
3. Orderbook updates `cache`; strategies read best bid/ask / depth from cache

### Order intent → venue

1. Strategy `SubmitOrder` → cache insert (`Initialized`) + `OrderRiskCheck` command
2. Risk `Checker` runs ordered rules (`ratelimit`, `tpnl`, …)
3. Pass → `OrderNew` event + `OrderSubmit` command; fail → `OrderRiskInvalid` (no submit)
4. OMS updates cache; execution engine submits via `ExecutionRouter`
5. Venue private stream → accept / fill / cancel / reject → OMS → strategies

Strategies never talk to venues directly; risk sits on the mandatory path between intent and submit.

## Project structure

```
seq/
├── cmd/                 # seq binary
├── config/              # YAML scenarios
├── node/                # composition root + event loop
├── core/
│   ├── actor|engine|msgbus|mem|clock|cache|catalog|config|logger|env|model
├── adapter/             # DataRouter, ExecutionRouter, binance/, bybit/
├── data/                # market-data engine + orderbook actor
├── execution/           # order engine + oms actor
├── ledger/           # balance engine + balance actor
├── risk/                # risk engine, checker, rules, ratelimiter/tpnl actors
└── strategy/            # strategy engine + xarb/obtest (+ StrategyActorBase)
```

## Development

```bash
make build-local      # local platform binary
make build            # linux/amd64 binary
make test             # go test -race ./...
make test-coverage    # coverage HTML
make benchmark
make fmt
make vet
make lint             # golangci-lint
make clean
make help
```

CI: `.github/workflows/go.yml`.

## Observability

| Mechanism | Coverage |
| --- | --- |
| Zerolog structured logs | Ops / debug |
| Msgbus msglog (`.jsonl`) | Plaintext event + command audit trail |
| Engine state events | Ready / stop / abnormal fan-out |

Not yet first-class: metrics/latency dashboards, kill-switch service, deterministic backtest harness. Paper-vs-live execution gate is in place (`trading_mode`). See `architecture.md` §9 and §13 for gaps and suggested follow-ups.

## License

See [LICENSE](./LICENSE).
