package telemetry

import (
	"fmt"
	"net/http"
	"runtime/metrics"
	"time"

	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
)

// MetricsConfig configures the metrics HTTP server (P2-4 step 5).
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Addr    string `yaml:"addr"` // e.g. "127.0.0.1:9100"
}

// runtimeHistograms are the runtime/metrics distributions exposed as
// percentile summaries.
var runtimeHistograms = []string{
	"/gc/pauses:seconds",
	"/sched/latencies:seconds",
}

// runtimeGauges are scalar runtime/metrics values exposed verbatim.
var runtimeGauges = []string{
	"/gc/cycles/total:gc-cycles",
	"/memory/classes/heap/objects:bytes",
	"/memory/classes/total:bytes",
	"/sched/goroutines:goroutines",
}

// StartMetricsServer serves the observability endpoints on cfg.Addr:
//
//	GET  /metrics  plain-text dump: msgbus drop/wait/unrouted counters and
//	               runtime/metrics gauges + histogram percentiles
//	               (p50/p99/p99.9/max), including /gc/pauses:seconds and
//	               /sched/latencies:seconds.
//	POST /gc       force a garbage collection (quiet-window hook for
//	               gc_off deployments).
//
// The server runs on its own goroutines and never touches the hot path:
// counters are read with atomic loads on request.
func StartMetricsServer(cfg MetricsConfig, bus *msgbus.MsgBus) (*http.Server, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	addr := cfg.Addr
	if addr == "" {
		addr = "127.0.0.1:9100"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		writeBusCounters(w, bus)
		writeRuntimeMetrics(w)
	})
	mux.HandleFunc("/gc", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		start := time.Now()
		ForceGC()
		fmt.Fprintf(w, "gc forced in %s\n", time.Since(start))
	})

	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// Startup failure (e.g. port in use) is a lifecycle event.
			fmt.Printf("telemetry: metrics server failed: %v\n", err)
		}
	}()
	return srv, nil
}

func writeBusCounters(w http.ResponseWriter, bus *msgbus.MsgBus) {
	if bus == nil {
		return
	}
	for t := event.Topic(0); t < event.TopicCount; t++ {
		if drops := bus.DropCount(t); drops != 0 {
			fmt.Fprintf(w, "seq_events_dropped_total{topic=%q} %d\n", t.String(), drops)
		}
		if waits := bus.WaitCount(t); waits != 0 {
			fmt.Fprintf(w, "seq_events_overflow_waits_total{topic=%q} %d\n", t.String(), waits)
		}
	}
	fmt.Fprintf(w, "seq_commands_unrouted_total %d\n", bus.UnroutedCommandCount())
}

func writeRuntimeMetrics(w http.ResponseWriter) {
	samples := make([]metrics.Sample, 0, len(runtimeGauges)+len(runtimeHistograms))
	for _, name := range runtimeGauges {
		samples = append(samples, metrics.Sample{Name: name})
	}
	for _, name := range runtimeHistograms {
		samples = append(samples, metrics.Sample{Name: name})
	}
	metrics.Read(samples)

	for _, s := range samples {
		switch s.Value.Kind() {
		case metrics.KindUint64:
			fmt.Fprintf(w, "go %s %d\n", s.Name, s.Value.Uint64())
		case metrics.KindFloat64:
			fmt.Fprintf(w, "go %s %g\n", s.Name, s.Value.Float64())
		case metrics.KindFloat64Histogram:
			writeHistogramSummary(w, s.Name, s.Value.Float64Histogram())
		}
	}
}

// writeHistogramSummary emits count plus approximate p50/p99/p99.9/max for a
// runtime/metrics histogram (bucket upper bounds; max is the upper bound of
// the highest non-empty bucket).
func writeHistogramSummary(w http.ResponseWriter, name string, h *metrics.Float64Histogram) {
	var total uint64
	for _, c := range h.Counts {
		total += c
	}
	fmt.Fprintf(w, "go %s count %d\n", name, total)
	if total == 0 {
		return
	}
	for _, q := range []struct {
		label string
		q     float64
	}{{"p50", 0.50}, {"p99", 0.99}, {"p99.9", 0.999}} {
		fmt.Fprintf(w, "go %s %s %g\n", name, q.label, histogramQuantile(h, q.q, total))
	}
	for i := len(h.Counts) - 1; i >= 0; i-- {
		if h.Counts[i] != 0 {
			fmt.Fprintf(w, "go %s max %g\n", name, h.Buckets[i+1])
			return
		}
	}
}

func histogramQuantile(h *metrics.Float64Histogram, q float64, total uint64) float64 {
	target := uint64(q * float64(total))
	var cum uint64
	for i, c := range h.Counts {
		cum += c
		if cum > target {
			return h.Buckets[i+1] // upper bound of the containing bucket
		}
	}
	return h.Buckets[len(h.Buckets)-1]
}
