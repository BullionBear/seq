# Bybit market data adapter

Scope: **spot** public market data as implemented in `adapter/bybit` (`BybitDataClient`). The client can open per-category public sockets (`spot` / `linear` / `inverse` / `option`), but the router today only registers a **spot** factory.

Official docs (source of the venue column below):

- [Connect](https://bybit-exchange.github.io/docs/v5/ws/connect)
- [Orderbook](https://bybit-exchange.github.io/docs/v5/websocket/public/orderbook)
- [Trade](https://bybit-exchange.github.io/docs/v5/websocket/public/trade)
- [Kline](https://bybit-exchange.github.io/docs/v5/websocket/public/kline)
- [Ticker](https://bybit-exchange.github.io/docs/v5/websocket/public/ticker)
- REST: [`GET /v5/market/orderbook`](https://bybit-exchange.github.io/docs/v5/market/orderbook), [`GET /v5/market/kline`](https://bybit-exchange.github.io/docs/v5/market/kline)

---

## Design note: normalize per stream, not a unified venue API

Seq does **not** expose a Bybit-shaped “subscribe everything” facade.

`adapter.DataClient` is a thin subscription/request surface. Each Bybit topic handler maps V5 JSON **directly** onto shared msgbus events:

| Venue topic | Message `type` | Normalized event |
| --- | --- | --- |
| `orderbook.{depth}.{symbol}` | `snapshot` | `TopicEventDepthSnapshot` |
| `orderbook.{depth}.{symbol}` | `delta` | `TopicEventDepthUpdate` |
| REST `/v5/market/orderbook` | — | `TopicEventDepthSnapshot` |
| `publicTrade.{symbol}` | (trade array) | `TopicEventTick` |
| `kline.{interval}.{symbol}` | `snapshot` | `TopicEventKline` |
| REST `/v5/market/kline` | — | `TopicEventRespHistoricalKline` |

Downstream actors never see Bybit field names (`confirm`, `S`, `u`, …) — only common `core/model/event` types.

---

## Endpoints we use

| Kind | URL |
| --- | --- |
| Public WS | `wss://stream.bybit.com/v5/public/{spot\|linear\|inverse\|option}` |
| REST | `https://api.bybit.com` |

Configured in `adapter/bybit/const.go`. Testnet / regional hosts (`bybit_tr`, `bybit_eu`) are **not** wired. YAML `datarouter.endpoint` is unused.

Subscribe op:

```json
{ "op": "subscribe", "args": ["orderbook.50.ETHUSDT", "kline.1.ETHUSDT"] }
```

---

## Feature matrix (venue market-data vs Seq)

Legend: **Yes** = implemented · **Partial** = subset · **No** = venue offers it, we do not

### WebSocket public topics

| Venue topic | Venue notes | Seq | Normalized to |
| --- | --- | --- | --- |
| Orderbook `orderbook.{1\|50\|200\|1000}.{symbol}` | Snapshot then deltas; push rate tied to depth | **Yes** (`depth.levels`) | `DepthSnapshot` / `DepthUpdate` |
| Orderbook option depths `25` / `100` | Option category | **Partial** — constants exist; no option factory in router | — |
| Orderbook full-depth variants | Venue docs mention full/resync patterns beyond standard depths | **No** | — |
| Public trade `publicTrade.{symbol}` | Up to many trades per message; taker side `Buy`/`Sell` | **Yes** | `Tick` |
| Aggregate trade | Binance-style aggTrade | **N/A** on Bybit; YAML `aggTrade` **ignored** | — |
| Kline `kline.{interval}.{symbol}` | `confirm=true` ⇒ closed; push ~1–60s | **Yes** (interval subset) | `Kline` (`Closed` ← `confirm`; `TradeCount=0`) |
| Ticker `tickers.{symbol}` | BBO + 24h stats (spot snapshot-only) | **No** | — |
| Liquidation / LT / other public topics | | **No** | — |

### REST market data

| Venue endpoint | Seq | Normalized to |
| --- | --- | --- |
| `GET /v5/market/orderbook` | **Yes** (`ReqDepthSnapshot`) | `DepthSnapshot` |
| `GET /v5/market/kline` | **Yes** (`ReqHistoricalKline`; list reversed oldest→newest) | `RespHistoricalKline` |
| Other `/v5/market/*` (tickers, recent trade, instruments-info, …) | **No** | — |

### Product / deployment coverage

| Capability | Seq |
| --- | --- |
| Spot public WS + REST | **Yes** (factory registered) |
| Linear / inverse / option public WS | **Partial** — client can open category sockets if symbols were routed; **no** linear/inverse/option factories in `DataRouter` today |
| Regional / testnet hosts | **No** |

---

## Config → topic mapping

```yaml
datarouter:
  - symbol: BYBIT_SPOT_ETHUSDT
    depth:
      levels: 50           # 1 | 50 | 200 | 1000 → orderbook.{n}.{SYMBOL}
      # type / push_rate: not used for Bybit push cadence (level-implied)
    trade: {}              # always publicTrade.{SYMBOL}
    kline:
      interval: 1m         # Binance-style string; mapped to Bybit tokens
```

### Depth levels and push frequency (venue)

| Level | Spot push (venue docs / code comments) |
| --- | --- |
| 1 | 10ms |
| 50 | 20ms |
| 200 | 200ms spot (100ms linear/inverse in venue docs) |
| 1000 | 200ms |

Unlike Binance, there is no separate “partial vs diff” stream type: one `orderbook.N` topic delivers snapshot + delta. Seq maps those message types to `DepthSnapshot` / `DepthUpdate` respectively.

**Config pitfall:** Binance-style `depth.type: depth5` is translated by the router into `depthLevel=5` before calling Bybit. That yields `orderbook.5.*`, which is **not** a valid Bybit spot depth. Prefer `levels:` only for Bybit symbols.

---

## Kline intervals

| Config (`interval`) | Bybit token | Seq |
| --- | --- | --- |
| `1m` … `30m` (and `1`,`3`,…) | `1`…`30` | **Yes** |
| `1h`/`60`, `2h`/`120`, `4h`/`240`, `6h`/`360`, `12h`/`720` | same | **Yes** |
| `1d`/`D`, `1w`/`W`, `1M`/`M` | `D`/`W`/`M` | **Yes** |
| `1s` | — | **No** (`BybitTopic` rejects) |
| `8h`, `3d` | — | **No** |

---

## Local order book

Venue: consume `orderbook.N` snapshot, then apply deltas (`u` / `seq`); treat `u=1` after snapshot as service restart overwrite.

Seq: adapter publishes snapshot/delta events; `data/actor/orderbook` maintains the cache book and can REST-resync via `ReqDepthSnapshot`. Adapter does not hold the book.

---

## Key files

| File | Role |
| --- | --- |
| `adapter/bybit/dataclient.go` | Per-category WS, topic subscribe, per-stream normalize/publish |
| `adapter/bybit/http.go` | REST orderbook + hist kline |
| `adapter/bybit/model.go` | Depth level constants / helpers |
| `adapter/bybit/const.go` | Base URLs / paths |
| `core/model/common/interval.go` | `Interval.BybitTopic()` mapping |
| `adapter/datarouter.go` | YAML → `DataClient` primitives |

---

## Gaps worth knowing

1. No `tickers.*` (BBO / 24h stats without full book).
2. No aggTrade equivalent; YAML `trade.type: aggTrade` is ignored.
3. `push_rate` ignored — cadence comes from orderbook depth.
4. Spot-only factory in the router despite multi-category client code.
5. `datarouter.endpoint` unused (no TR/EU/testnet switching).
6. Kline WS sets `TradeCount=0` (field not in Bybit kline payload).
