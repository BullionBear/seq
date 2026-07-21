# Binance USD-M Futures adapter

Scope: **USD-M perpetual** market data and execution in `adapter/binancefutures`
(`BinanceFuturesDataClient`, `BinanceFuturesExecutionClient`).

COIN-M (`dapi` / `dstream`) is out of scope for this phase.

Spot Binance lives in [`adapter/binance`](../binance/BINANCE.md) and is unchanged.

Official docs:

- [USD-M Futures WebSocket Streams](https://developers.binance.com/docs/derivatives/usds-margined-futures/websocket-market-streams)
- REST: [`GET /fapi/v1/depth`](https://developers.binance.com/docs/derivatives/usds-margined-futures/market-data/rest-api/Order-Book), [`GET /fapi/v1/klines`](https://developers.binance.com/docs/derivatives/usds-margined-futures/market-data/rest-api/Kline-Candlestick-Data)
- [USD-M Futures WebSocket API](https://developers.binance.com/docs/derivatives/usds-margined-futures/websocket-api-general-info)

---

## Design note

Same as spot: `adapter.DataClient` / `ExecutionClient` are thin venue surfaces.
Parsers map venue JSON directly onto shared msgbus events. Downstream actors never
see Binance payload shapes.

| Venue stream / event | Normalized event |
| --- | --- |
| `@depth@…` (diff, includes `pu`) | `TopicEventDepthUpdate` |
| `@depth5\|10\|20@…` (partial) | `TopicEventDepthSnapshot` |
| REST `/fapi/v1/depth` | `TopicEventDepthSnapshot` |
| `@trade` / `@aggTrade` | `TopicEventTick` |
| `@kline_*` | `TopicEventKline` |
| REST `/fapi/v1/klines` | `TopicEventRespHistoricalKline` |
| `ORDER_TRADE_UPDATE` | order lifecycle + `Execution` |
| `ACCOUNT_UPDATE` | `BalanceUpdate` |

---

## Endpoints

| Kind | URL |
| --- | --- |
| Combined / raw public WS | `wss://fstream.binance.com` |
| REST | `https://fapi.binance.com` |
| Trading WS API | `wss://ws-fapi.binance.com/ws-fapi/v1` |

Configured in `const.go`. Testnet hosts are **not** wired.

---

## Selection / wiring

| Path | Key |
| --- | --- |
| Data | Catalog symbol `(exchange=Binance, product=PERPETUAL)` → `DataRouter` factory |
| Exec | `account.exchange: Binance` + `execrouter.wallet` type `umargin` → futures client |

Example instrument: `BINANCE_PERPETUAL_ETHUSDT` in `config/instruments.json`.

```yaml
datarouter:
  - symbol: BINANCE_PERPETUAL_ETHUSDT
    depth:
      type: delta
      push_rate: 100ms   # futures: 100ms | 250ms | 500ms (>=1000ms → 500ms)
    trade: {}              # → {symbol}@trade
    aggTrade: {}           # → {symbol}@aggTrade (independent of trade)
    kline:
      interval: 1m

catalog:
  accounts:
    - name: binance_main
      exchange: Binance
      wallets:
        - name: um
          type: umargin
      # api_keys: ...

execrouter:
  - account: binance_main
    wallet: um
    api: trading
```

---

## Depth push rates

Futures supports `100ms`, `250ms`, and `500ms` (not spot's `1000ms`).
YAML `push_rate: 1000ms` / `1s` maps to `500ms`.

Diff depth continuity uses venue `pu` (previous final update ID) for
`PreviousDepthID` when present.

---

## Execution notes

- Post-only (`TimeInForcePO`) → `LIMIT` + `timeInForce=GTX` (not spot `LIMIT_MAKER`).
- User data: `ORDER_TRADE_UPDATE` / `ACCOUNT_UPDATE` (not spot `executionReport`).
- Balance snapshot: WS API `v2/account.balance`.
- Ed25519 signing + IPv4 dialer (same constraints as spot).

---

## Key files

| File | Role |
| --- | --- |
| `dataclient.go` | Public fstream + normalize/publish |
| `http.go` | REST depth + hist kline |
| `execclient.go` | ws-fapi trading + user data |
| `const.go` / `model.go` | URLs, push-rate helpers |
| `adapter/datarouter.go` | Registers Binance + Perpetual factory |
| `node/node.go` | Dispatches umargin → futures exec client |

---

## Gaps

1. COIN-M not implemented.
2. No bookTicker / markPrice / funding streams.
3. Testnet hosts unused.
4. `datarouter.endpoint` unused.
