# Adding a new event type

End-to-end checklist for introducing a new msgbus **event**. Commands follow a parallel path (see §9).

Reference implementations:

| Kind | Example | Files |
| --- | --- | --- |
| Fixed-size POD | `Kline`, `OrderFilled` | `core/model/event/kline.go`, `kline_codec.go`, `order.go`, `order_codec.go` |
| Variable-size (array) | `RespHistoricalKline`, `DepthSnapshot` | `kline_hist_codec.go`, `depth_codec.go` |
| Variable-size (string) | `OrderError` | `order_string_codec.go` |

---

## 1. Decide layout and overflow class

1. **Fixed-size POD** if every field is a numeric/enum — no `string`, `[]T`, pointer, map, or interface. Prefer this.
2. **Variable-size** if you need a trailing array (bars, price levels) or a UTF-8 message.
3. **Droppable vs critical** (`Topic.IsDroppable`):
   - Droppable: recoverable market data (depth, tick, live kline, timer) — may be dropped under overflow.
   - Critical: order lifecycle, balances, engine state, hist-kline responses — wait then **fatal**; never drop.

---

## 2. Define the struct

Create or extend a file under `core/model/event/`:

```go
// Fixed-size example — largest fields first; no pointers/slices/strings.
type MyEvent struct {
    SymbolID  int
    Timestamp uint64
    Price     float64
    Qty       float64
}

func (e MyEvent) Topic() Topic { return TopicEventMyEvent }
```

For variable-size events, document that slice fields may **alias the arena** and are only valid until `Handle` returns (see `RespHistoricalKline.Bars`).

---

## 3. Add the topic

In `core/model/event/topic.go`:

1. **Append** the constant **before** `TopicCount` (never insert mid-block — iota values must stay stable).
2. Update `String()`, `topicByName`, and `IsDroppable()` if the topic is droppable.

```go
// Market Data (appended: keep prior iota values stable)
TopicEventMyEvent

TopicCount // must remain last
```

---

## 4. Implement the codec

### Fixed-size

`core/model/event/myevent_codec.go`:

```go
func (e MyEvent) GetBufferLength() int { return codec.Size[MyEvent]() }
func (e MyEvent) Encode(buf []byte) error { return codec.Encode(buf, &e) }
func NewMyEventFromBytes(buf []byte) (MyEvent, error) { return codec.Decode[MyEvent](buf) }
```

### Variable-size (array of PODs)

Follow `kline_hist_codec.go` / `depth_codec.go`:

1. POD header + trailing element array.
2. `GetBufferLength()` = header size + `n * elementSize`.
3. `Encode` via `codec.NewCursor` + `codec.Put` for each element.
4. `New*FromBytes` validates length; zero-copy views into `buf` are OK **only inside `Handle`**.

Register **element** PODs (e.g. `KlineBar`, `PriceLevel`) in the wire-type registry — not the outer wrapper.

---

## 5. Register the wire type (CI layout guard)

Add every type passed to `codec.Encode` / `Decode` / `Put` to `wireTypes` in
`core/model/codec/registry_test.go` (sizes + field offsets). Layout changes must bump those goldens deliberately — see `core/model/codec/doc.go`.

---

## 6. Wire msglog decoding

In `core/msgbus/msglog.go` → `decodeEventPayload`, add a case:

```go
case event.TopicEventMyEvent:
    return orDecodeErr(event.NewMyEventFromBytes(buf))
```

Without this, JSONL logs only `{"raw_len": N}` for that topic.

---

## 7. Publish (adapters / engines)

```go
size := uint64(ev.GetBufferLength())
ref, buf, ok := msgBus.Allocate(event.TopicEventMyEvent, size)
if !ok {
    return // droppable: drop; critical: Allocate fatals after deadline
}
if err := ev.Encode(buf); err != nil {
    msgBus.Cancel(ref)
    return
}
msgBus.Publish(ref)
```

Always `Cancel(ref)` if encode fails after a successful `Allocate`.

---

## 8. Consume (actors)

1. Subscribe at construction (topics list on `NewStrategyActorBase` / `SubscribedTypes`).
2. In `Handle`:

```go
case event.TopicEventMyEvent:
    buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
    msg, err := event.NewMyEventFromBytes(buf)
    if err != nil {
        return
    }
    // use msg; copy out any zero-copy slices before returning
```

---

## 9. Commands (same idea, different APIs)

| Events | Commands |
| --- | --- |
| `Topic` + `Topic()` | `CommandType` in `command/type.go` + `CommandType()` |
| `Allocate` / `Publish` | `AllocateCmd` / `Send` (dispatch thread only) |
| Actor topic fan-out | Exactly one `RegisterCommand` processor |
| `decodeEventPayload` | `decodeCommandPayload` |
| Same fixed-size `codec` + `wireTypes` | Same |

Request/response pairs are common (e.g. `ReqHistoricalKline` → `RespHistoricalKline`).

---

## 10. Tests

Minimum:

1. Round-trip Encode → `New*FromBytes` (see `kline_hist_codec_test.go`).
2. Short / malformed buffer returns error (no panic).
3. Wire-type registry entry so POD/layout guards cover the type.
4. Optional: msglog decode case in `core/msgbus/msglog_test.go`; adapter publish/poll test.

---

## Checklist

- [ ] Struct + `Topic()` in `core/model/event/`
- [ ] Topic constant appended before `TopicCount`; `String` / `topicByName` / `IsDroppable`
- [ ] Codec (`*_codec.go`) — fixed or variable
- [ ] `wireTypes` registry entry for every POD / array element
- [ ] Msglog `decodeEventPayload` case
- [ ] Publisher: `Allocate` → `Encode` → `Publish` (Cancel on encode error)
- [ ] Consumer: subscribe topic + `ReadBuffer` + `New*FromBytes`
- [ ] Copy out zero-copy slices before `Handle` returns
- [ ] Round-trip (+ fuzz/malformed) tests

---

## Pitfalls

1. **Reordering topics** breaks on-disk/msglog topic IDs — only append.
2. **Wrong droppable class** either drops critical state or fatals on market-data bursts.
3. **Storing arena-aliased slices** after `Handle` returns → torn / reused memory.
4. **Skipping msglog case** → opaque `raw_len` audit trail.
5. **Skipping registry** → no CI protection when someone adds a `string` field or reorders layout.
