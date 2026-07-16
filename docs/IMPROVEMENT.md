# IMPROVEMENT.md — seq Runtime Hardening Plan

Scope: `core/mem`, `core/msgbus`, `core/model`, `node`, `adapter/*`.
Goal: eliminate silent data-loss and torn-read hazards, drive steady-state heap
allocation on the hot path to zero, and reduce codec boilerplate to a single
source of layout truth — while keeping the current single-consumer dispatch
architecture intact.

Tasks are ordered by priority within each phase. Every task is independently
mergeable and independently verifiable. Task IDs are stable; reference them in
commit messages (e.g. `fix(msgbus): P0-1 spin on full order-event ring`).

---

## Phase 0 — Correctness (fix before any performance work)

### P0-1 · Silent event loss on full event ring

**Location:** `core/msgbus/eventbus.go` — `EventBus.Publish` (~line 183)

**Problem (hidden bug):**
`Publish` ignores the boolean return of `rbEvent.Write`. When the MPSC ring is
full, `Write` returns `false` and the event is **silently dropped**. Market
data events are recoverable via snapshot re-request; order-lifecycle events
(`OrderNew`, `OrderFilled`, `OrderCanceled`, …) are not — a dropped fill
diverges local state from venue state until the next reconciliation, i.e. the
system trades on wrong positions/balances.

**Fix conditions:**
1. `Publish` must not discard the `Write` result.
2. Introduce per-topic (or per-class) overflow policy:
   - **Critical class** (order, balance, engine-state events): producer spins
     (bounded spin with `runtime.Gosched()` escalation) until a slot frees, or
     fails hard after a configurable deadline with a fatal state notification.
     Never drop.
   - **Droppable class** (depth updates, ticks): drop is permitted, but must
     increment an atomic per-topic drop counter and the consumer-side recovery
     path (snapshot re-request) must be triggered.
3. Drop/spin counters exported via a metrics hook (see P2-4).

**Verification:**
- Unit test: fill the ring to capacity from N producer goroutines, assert
  (a) zero loss for critical topics (all EventIDs observed exactly once,
  in per-producer order), (b) drop counter equals `published - consumed` for
  droppable topics.
- Chaos test: pause the dispatch goroutine for 100 ms under sustained publish
  load; assert no critical event is missing after resume.
- `go test -race` clean on the new paths.

---

### P0-2 · Torn reads in `CircularByteArena` (advisory overwrite protection)

**Location:** `core/mem/circular.go` — `Reserve`, `checkOverwrite`, `ReadSlice`

**Problem (hidden bug):**
Overwrite protection is explicitly "advisory / best effort": when a producer's
reservation overlaps unread data, the code logs a warning and CAS-advances
`readOff`, but `ReadSlice` returns an unvalidated slice aliasing the buffer.
Under burst, the consumer can read a region a producer is concurrently
overwriting — a **torn read** that silently corrupts a decoded event. The
wrap-to-zero policy in `Reserve` (jump to offset 0 when the tail is short)
makes this more likely: a writer can wrap onto the region the reader is in the
middle of.

**Fix conditions (choose one mechanism, apply to both event and command arenas):**
- **Option A (preferred, Disruptor-style):** make the arena bounded by the
  consumer. `Reserve` must not proceed past the consumer's released offset;
  producers wait (same policy classes as P0-1). Overwrite becomes impossible
  by construction; `checkOverwrite` is deleted.
- **Option B (seqlock-style):** prefix each allocation with an 8-byte sequence
  word. Producer bumps it to odd before writing and even after; consumer
  validates the word before and after copying the payload out, retrying on
  mismatch. Read becomes copy-out (no aliasing).

In both options: document (in the type's doc comment) the exact producer /
consumer visibility contract, and remove the "best effort" language.

**Verification:**
- Stress test: 8 producers × small arena (force wraps every few hundred
  writes) × checksum-carrying payloads; consumer validates checksum of every
  event. Zero mismatches over ≥ 10⁸ events.
- `go test -race` clean.
- Loom-style interleaving unit test for the wrap path (producer wraps while
  consumer holds a slice / is mid-copy).

---

### P0-3 · Unchecked, unaligned decode from arena

**Location:** `core/model/*/**_codec.go` — all `New*FromBytes` functions;
`core/mem/circular.go` — `Reserve`

**Problem (hidden bug):**
`New*FromBytes` does `*(*T)(unsafe.Pointer(&buf[0]))` with **no length
check** — a short slice reads out of bounds. It also type-punns an arbitrary
arena offset: `Reserve(size)` performs no alignment, so the resulting load can
be unaligned (slow on amd64; faulting on other targets, and incompatible with
any future atomic field).

**Fix conditions:**
1. All decode paths bounds-check before reading (subsumed by P1-1's generic
   `Decode`, but must land even if P1-1 is deferred).
2. Decode copies out of the arena into a local value instead of pointer-casting
   into it (also required for P0-2 Option B).
3. `Reserve` rounds every allocation size up to 8-byte alignment; add an
   invariant check (debug build) that returned offsets are 8-aligned.

**Verification:**
- Fuzz test (`go test -fuzz`): feed truncated/oversized/garbage buffers to
  every `Decode`/`New*FromBytes`; must return an error, never panic or read OOB.
- Unit test asserting `Reserve` offset alignment across wrap boundaries.
- Run the full test suite under `GOARCH=arm64` (CI matrix) to catch alignment
  regressions.

---

### P0-4 · Msglog records carry no version or layout contract

**Location:** `core/msgbus/msglog.go`, `cmd/parser`, `core/model`

**Problem (hidden bug):**
The `.dat` wire format is currently *whatever the Go compiler laid out*:
`unsafe.Sizeof` whole-struct memcpy includes implicit padding, and there is no
record version. Reordering one struct field, or a compiler layout change,
silently invalidates all historical logs; the parser has no way to detect it.

**Fix conditions:**
1. Prepend a per-record header: `{schemaVersion u16, msgType u16, length u32}`.
2. Prepend a per-file header: magic, endianness marker, build/commit,
   schema version.
3. `cmd/parser` refuses (with a clear error) files whose schema version it
   does not support.
4. Struct layout becomes an explicit contract (see P1-2 guard tests).

**Verification:**
- Round-trip test: write a log with the current version, mutate the schema
  version constant, assert parser rejects with the expected error.
- Golden-file test: a checked-in `.dat` fixture decodes to a checked-in JSONL
  fixture byte-for-byte.

---

## Phase 1 — Codec consolidation (kill the boilerplate, keep the format)

### P1-1 · Collapse fixed-size codecs into one generic Encode/Decode pair

**Location:** new `core/model/codec` package; delete per-type boilerplate in
`*_codec.go`

**Problem:**
Every fixed-size type repeats the same ten lines (`GetBufferLength`, `Encode`
via `unsafe` memcpy, `New*FromBytes`). ~1,800 lines of codecs encode zero
type-specific knowledge.

**Fix conditions:**
1. Implement:
   ```go
   func Encode[T any](buf []byte, v *T) error   // bounds-checked memcpy out of v
   func Decode[T any](buf []byte) (T, error)    // bounds-checked memcpy into local
   ```
2. Migrate all fixed-size event/command types; delete their hand-written
   codecs. Variable-size types are out of scope here (P1-3).
3. No change to the wire layout in this task (byte-for-byte identical output),
   so it is independently mergeable before/after P0-4.

**Verification:**
- Differential test: for every migrated type, `oldEncode(v) == newEncode(v)`
  byte-for-byte over randomized values (keep old functions in `_test.go` until
  migration completes).
- `testing.AllocsPerRun` == 0 for both functions.
- Line-count check: `core/model/**/*_codec.go` net reduction recorded in the PR.

---

### P1-2 · POD and layout guard tests

**Location:** `core/model/codec/guard_test.go` (new)

**Problem:**
The memcpy format is only sound for POD structs with a frozen layout. Nothing
currently prevents someone adding a `string` field or reordering fields.

**Fix conditions:**
1. A registry of all wire types; a reflect-based test walks each recursively
   and fails on any pointer, slice, map, string, chan, func, or interface field.
2. Golden layout test: assert `unsafe.Sizeof` and `unsafe.Offsetof` of every
   field of every wire type against checked-in constants. Any layout change
   fails CI until the constants (and msglog schema version, per P0-4) are
   bumped deliberately.
3. Style rule (documented in the package doc): wire structs order fields
   largest-first and spell out padding as `_ [N]byte` so layout is declared,
   not inherited.

**Verification:**
- The guard tests themselves; plus a canary PR that reorders one field must
  fail CI.

---

### P1-3 · Zero-copy views + write cursor for variable-size types

**Location:** `core/model/event/depth*.go` (and any future variable-size type)

**Problem:**
`DepthSnapshot`/`DepthUpdate` are hand-walked with `pos += 8` arithmetic and
hand-computed header constants (`DepthSnapshotHeaderSize = 32`,
`DepthUpdateHeaderSize = 56`) that must be kept in sync with the comment by
eye. Decoding into structs with `Asks/Bids []PriceLevel` either aliases the
arena (unsafe post-P0-2) or allocates.

**Fix conditions:**
1. Read path: flyweight views over the buffer —
   `DepthSnapshotView{buf []byte}` with accessor methods (`SymbolID()`,
   `NumAsks()`, `Ask(i)`), no materialization. Consumers (orderbook actor)
   iterate the view directly.
2. Write path: a small `Cursor` writer (position + sticky error) replacing raw
   offset arithmetic.
3. Header sizes derived from `unsafe.Offsetof`/`Sizeof` or emitted constants —
   no hand-computed magic numbers remain (grep-able acceptance: no numeric
   literal header sizes in the package).
4. View constructors validate minimum length and the
   `header + n*PriceLevelSize` invariant (ties into P0-3 fuzzing).

**Verification:**
- Differential test vs. the current encoder (byte-identical output).
- `AllocsPerRun` == 0 for encode and for a full view iteration.
- Fuzz the view constructor with malformed lengths.

---

### P1-4 · (Deferred decision) schema-driven code generation

**Trigger condition:** next time a batch of new event/command types is added,
or when a non-Go consumer (research tooling) needs to read the msglog.

**Options to evaluate:** SBE (matches the Aeron/LMAX lineage; flyweights +
built-in schema evolution; adds a Java tool to the build), or an in-repo
~200-line generator (`go/ast` + `text/template`) emitting the P1-1/P1-3 style
code from struct definitions.

**Acceptance for the evaluation itself:** a one-page ADR comparing the two
against: build-chain cost, msglog evolution story, cross-language readers.
No code change required by this task.

---

## Phase 2 — Hot-path allocation and scheduling (GC starvation)

Strategy: the Go GC is allocation-rate triggered. The objective is not tuning
GC but starving it — steady-state allocations/op = 0 on the dispatch path and
per-message = 0 on adapter read paths — then fencing the remainder.

### P2-1 · Zero-allocation WebSocket frame reads

**Location:** `adapter/binance/dialer.go`, `adapter/bybit/dialer.go`

**Problem:**
`ReadMessage()`-style APIs allocate a fresh `[]byte` per frame — the single
largest steady-state allocation source.

**Fix conditions:**
1. Each connection goroutine owns one reusable read buffer (grow-only,
   high-water sized). Frames are read into it via `NextReader` + `io.ReadFull`
   (gorilla) or by switching the dialer to `gobwas/ws` with caller-managed
   buffers. One buffer per connection suffices (reads are sequential per
   connection).
2. The buffer's lifetime contract: contents are valid only until the next
   read; parsing (P2-2) and arena encode complete within that window.

**Verification:**
- Benchmark with `b.ReportAllocs()` on the frame-read + parse + publish path
  using recorded frames: **0 allocs/op**.
- 10-minute soak against live testnet streams with `GODEBUG=gctrace=1`:
  GC cycle count for the adapter phase reduced to ~0 relative to baseline
  (record baseline first).

---

### P2-2 · Allocation-free JSON field extraction

**Location:** `adapter/binance/dataclient.go`, `adapter/bybit/dataclient.go`

**Problem:**
`jsonparser.GetString` performs a `string()` conversion — one heap allocation
plus copy per call, several per message (`"stream"`, `"s"`, numeric fields
parsed via string).

**Fix conditions:**
1. Hold `[]byte` end-to-end. Symbol lookup uses the map-key special case
   `m[string(b)]` (compiler-optimized, non-allocating) against a
   `map[string]SymbolID`; after resolution, only integer IDs cross into the
   arena.
2. Numeric fields: parse directly from the byte subslice (fixed-point parser)
   instead of `GetString` + `ParseFloat`. This dovetails with any future move
   to integer ticks.
3. String data must not escape the connection read buffer (ties to P2-1's
   lifetime contract).

**Verification:**
- `AllocsPerRun` == 0 for `processMessage` over a corpus of recorded frames
  (all message types).
- `go build -gcflags='-m'` output reviewed in the PR: no escape of the read
  buffer or derived slices.
- Differential test: decoded fields identical to the current implementation
  over the corpus.

---

### P2-3 · Logging discipline on hot paths

**Location:** `core/mem/circular.go`, `core/msgbus`, adapters

**Problem:**
zerolog is allocation-free when a level is disabled, but hot-path call sites
build allocating arguments (formatted strings, `Interface()`), and
`checkOverwrite` logs at event rate under exactly the conditions where the
system is already stressed.

**Fix conditions:**
1. No log statement above Debug on: dispatch loop, ring/arena operations,
   per-message adapter paths. High-frequency conditions (overflow, drop,
   overwrite-wait) become atomic counters reported by a low-frequency
   observer goroutine (1 s cadence).
2. The binary msglog is the sole per-event record; text logging is for
   lifecycle and cold paths only. Document this rule in `core/logger`.

**Verification:**
- Static check in CI: a lint rule (forbidigo or a small analyzer) rejecting
  `log()` calls in the designated hot-path packages/files.
- Hot-path benchmarks from P2-1/P2-2 remain 0 allocs/op with logging compiled
  in at production level.

---

### P2-4 · Dispatch thread pinning and runtime fencing

**Location:** `node/node.go`, `cmd/main.go`, deployment docs

**Problem:**
The dispatch goroutine is scheduled like any other: no `LockOSThread`, so the
Go scheduler migrates it across Ps/cores (cache-affinity loss, uncontrolled
wake-up latency). GC background workers and other goroutines share its core.

**Fix conditions:**
1. `runtime.LockOSThread()` at the top of the dispatch goroutine.
2. Configurable idle strategy: current `Gosched` (default) vs. bounded
   busy-spin for latency-critical deployments.
3. After Phase 2 allocation work: run with `GOGC=off` + `GOMEMLIMIT=<cap>`
   as the supported production configuration (GC as a memory fuse, not a
   periodic event); optional manual `runtime.GC()` hook at declared quiet
   windows.
4. Deployment doc: `taskset`/`isolcpus` recipe for giving the dispatch thread
   an isolated core; `GOMAXPROCS` guidance.
5. Metrics hook exposing `runtime/metrics` histograms
   (`/gc/pauses:seconds`, `/sched/latencies:seconds`) plus the P0-1/P2-3
   counters.

**Verification:**
- Latency harness: publish→dispatch handoff latency histogram (HDR) before/
  after, on an isolated core; report p50/p99/p99.9/max over ≥ 10⁸ events.
  Acceptance: p99.9 improvement documented; max bounded (no multi-ms
  scheduler outliers) in the pinned + spin configuration.
- 1-hour soak with `GOGC=off` + `GOMEMLIMIT`: heap high-water stable
  (no growth trend), zero GC cycles in steady state, `/gc/pauses` empty.

---

## Phase 3 — Regression fencing (keep it fixed)

### P3-1 · Allocation and layout gates in CI

**Fix conditions:**
1. `AllocsPerRun == 0` tests for: generic Encode/Decode (P1-1), depth view
   iteration (P1-3), adapter `processMessage` (P2-2), publish/dispatch round
   trip.
2. P1-2 guard tests wired into the default test target.
3. Benchmark job (nightly or on-demand label) publishing ns/op and allocs/op
   trends for the ring buffer, arena, and codec benchmarks; alert on
   allocs/op > 0 or > 20 % ns/op regression.

**Verification:** the gates themselves; a deliberate canary regression
(temporary PR adding one allocation) must fail.

### P3-2 · Race, fuzz, and architecture matrix

**Fix conditions:**
1. CI matrix: `go test -race` on linux/amd64 and linux/arm64.
2. Fuzz targets from P0-3/P1-3 run for a fixed budget per merge to main;
   corpus checked in.
3. The P0-1/P0-2 stress tests run in a scheduled (not per-PR) job with high
   iteration counts.

**Verification:** green matrix; fuzz corpus grows over time without new
crashers.

---

## Suggested sequencing

```
P0-1 ─┐
P0-2 ─┼─ merge independently, in any order, ASAP
P0-3 ─┤
P0-4 ─┘
P1-1 → P1-2 → P1-3            (P1-2 depends on P1-1's registry; P1-3 independent of P1-1)
P2-1 → P2-2                   (P2-2 relies on P2-1's buffer lifetime contract)
P2-3, P2-4                    (independent; P2-4 step 3 gated on P2-1/P2-2 completion)
P3-1, P3-2                    (land the gates as each phase completes)
P1-4                          (decision task; trigger-based, not scheduled)
```

Definition of done for the plan: all P0 verifications green; hot-path
allocs/op = 0 enforced in CI; a 1-hour production-config soak shows zero
steady-state GC cycles and a bounded dispatch-latency max.