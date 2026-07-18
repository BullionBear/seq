// Package logger provides the process-wide zerolog singleton.
//
// # Logging discipline (P2-3)
//
// The plaintext msglog (core/msgbus.MsgLogger, JSONL) is the SOLE per-event
// record. Zerolog text logging exists for lifecycle transitions (startup,
// connect, disconnect, shutdown) and cold error paths only. The rules:
//
//  1. No log statement above Debug on hot paths: the dispatch loop,
//     ring-buffer and arena operations (core/mem, core/msgbus
//     Allocate/Publish/Dispatch/Send), and per-message adapter parse paths
//     (dataclient processMessage and everything it calls).
//  2. High-frequency conditions — event drops, overflow waits, unrouted
//     commands — are recorded as atomic counters at the point of occurrence
//     and surfaced by a low-frequency observer goroutine
//     (msgbus.StartObserver, 1 s cadence), never logged inline.
//  3. Fatal/Panic are permitted anywhere: they terminate the process, so
//     they cannot recur at event rate.
//  4. Debug is permitted on hot paths for diagnostics, because zerolog
//     short-circuits disabled levels without allocating; production runs at
//     Info or above. Do not compute log arguments outside the zerolog call
//     chain (no fmt.Sprintf, no Interface() of live structs).
//
// These rules are enforced statically by internal/lint (TestHotPathLogging),
// which rejects Info/Warn/Error calls in the designated hot-path functions
// and any logger import in core/mem.
package logger
