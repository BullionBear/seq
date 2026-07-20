# Binance market data adapter

Scope: **spot** public market data as implemented in `adapter/binance` (`BinanceSpotDataClient`).

Official docs (source of the venue column below):

- [WebSocket Streams](https://github.com/binance/binance-spot-api-docs/blob/master/web-socket-streams.md)
- REST: [`GET /api/v3/depth`](https://binance-docs.github.io/apidocs/spot/en/#order-book), [`GET /api/v3/klines`](https://binance-docs.github.io/apidocs/spot/en/#kline-candlestick-data)

---

## Design note: normalize per stream, not a unified venue API

Seq does **not** wrap Binance behind a rich venue-specific interface that mirrors every stream type.

`adapter.DataClient` is a thin subscription/request surface (depth / trade / kline + REST helpers). Each Binance stream parser maps venue JSON **directly** onto shared msgbus events:

| Venue stream | Normalized event |
| --- | --- |
| `@depth@…` (diff) | `TopicEventDepthUpdate` |
| `@depth5|10|20@…` (partial) | `TopicEventDepthSnapshot` |
| REST `/api/v3/depth` | `TopicEventDepthSnapshot` |
| `@trade` / `@aggTrade` | `TopicEventTick` |
| `@kline_*` | `TopicEventKline` |
| REST `/api/v3/klines` | `TopicEventRespHistoricalKline` |

Downstream actors never see Binance payload shapes — only common `core/model/event` types.

---

## Endpoints we use

| Kind | URL |
| --- | --- |
| Combined / raw WS | `wss://stream.binance.com:9443` (`/ws/…` or `/stream?streams=…`) |
| REST | `https://api.binance.com` |

Configured in `adapter/binance/const.go`. Regional / `data-stream.binance.vision` / microsecond `timeUnit` are **not** wired. YAML `datarouter.endpoint` is unused.

---

## Feature matrix (venue market-data vs Seq)

Legend: **Yes** = implemented · **Partial** = subset · **No** = venue offers it, we do not · **N/A** = not applicable

### WebSocket streams

| Venue stream | Venue notes | Seq | Normalized to |
| --- | --- | --- | --- |
| Diff. depth `@depth` / `@depth@100ms` / `@depth@1000ms` | Changed levels only; local book needs REST snapshot + sync | **Yes** | `DepthUpdate` |
| Partial book `@depth5|10|20[@100ms\|@1000ms]` | Top-N replace each push | **Yes** (`type: depth5\|10\|20`) | `DepthSnapshot` |
| Trade `@trade` | Every match | **Yes** (`trade.type: trade`) | `Tick` |
| Aggregate trade `@aggTrade` | Same price/time/taker aggregated | **Yes** (`trade.type: aggTrade`) | `Tick` |
| Kline `@kline_<interval>` | Open + closed (`k.x`); intervals include `1s`…`1M` | **Yes** | `Kline` |
| Kline with timezone offset | Venue offers `@kline_<interval>@+08:00` etc. | **No** | — |
| Book ticker `@bookTicker` | Best bid/ask | **No** | — |
| Mini ticker / all mini tickers | 24h rolling stats (lite) | **No** | — |
| Individual / all ticker | Full 24h ticker | **No** | — |
| Rolling window stats | e.g. `@ticker_1h` | **No** | — |
| Average price `@avgPrice` | | **No** | — |
| Block trade | | **No** | — |
| Reference price | | **No** | — |
| Live SUBSCRIBE/UNSUBSCRIBE on open connection | Venue supports dynamic ops | **Partial** — streams baked into connect URL; reconnect rebuilds | — |

### REST market data

| Venue endpoint | Seq | Normalized to |
| --- | --- | --- |
| `GET /api/v3/depth` | **Yes** (`ReqDepthSnapshot`, limit 1000 from data engine) | `DepthSnapshot` |
| `GET /api/v3/klines` | **Yes** (`ReqHistoricalKline`) | `RespHistoricalKline` |
| Other public REST (trades, aggTrades, ticker, exchangeInfo, …) | **No** | — |

### Product / deployment coverage

| Capability | Seq |
| --- | --- |
| Spot | **Yes** (factory registered) |
| USD-M / COIN-M futures market data | **No** |
| Testnet hosts | **No** (hardcoded mainnet URLs) |

---

## Config → stream mapping

```yaml
datarouter:
  - symbol: BINANCE_SPOT_ETHUSDT
    depth:
      type: delta          # default → {symbol}@depth@{100ms|1000ms}
      # type: depth5       # → {symbol}@depth5@…
      push_rate: 100ms     # or 1000ms / 1s
    trade:
      type: trade          # or aggTrade
    kline:
      interval: 1m         # Binance form; default 1m
```

Depth `levels` is ignored for Binance (Bybit-oriented). Partial types publish **snapshots**, not diffs — matching Binance partial-stream semantics.

---

## Kline intervals

Venue + `common.Interval`: `1s`, `1m`, `3m`, `5m`, `15m`, `30m`, `1h`, `2h`, `4h`, `6h`, `8h`, `12h`, `1d`, `3d`, `1w`, `1M`.

Stream name uses `Interval.String()` → `{symbol}@kline_1m`.

---

## Local order book (diff depth)

Venue recipe: subscribe diff depth → REST snapshot → apply updates with `U`/`u` continuity; resync on gap.

Seq: adapter publishes `DepthUpdate` / `DepthSnapshot`; `data/actor/orderbook` owns the book state machine and issues `ReqDepthSnapshot` on gaps. Adapter does not maintain the book itself.

---

## Key files

| File | Role |
| --- | --- |
| `adapter/binance/dataclient.go` | WS subscribe + per-stream normalize/publish |
| `adapter/binance/http.go` | REST depth + hist kline |
| `adapter/binance/model.go` | Push-rate / stream suffix helpers |
| `adapter/binance/const.go` | Base URLs / paths |
| `adapter/datarouter.go` | YAML → `DataClient` primitives |

---

## Gaps worth knowing

1. No bookTicker / ticker / avgPrice streams (BBO without full depth).
2. No futures market-data factories.
3. `datarouter.endpoint` and alternate hosts unused.
4. `TopicEventRespDepthSnapshot` exists in the topic enum but Binance REST publishes `TopicEventDepthSnapshot` (same as partial WS).
