package telemetry

import (
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/BullionBear/seq/core/msgbus"
)

func TestMetricsHandlerOutput(t *testing.T) {
	bus := msgbus.NewMsgBus()

	rec := httptest.NewRecorder()
	writeBusCounters(rec, bus)
	writeRuntimeMetrics(rec)

	body, _ := io.ReadAll(rec.Result().Body)
	out := string(body)

	for _, want := range []string{
		"seq_commands_unrouted_total 0",
		"/gc/pauses:seconds",
		"/sched/latencies:seconds",
		"/gc/cycles/total:gc-cycles",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("metrics output missing %q; got:\n%s", want, out)
		}
	}
}

func TestGCEndpointRequiresPost(t *testing.T) {
	srv, err := StartMetricsServer(MetricsConfig{Enabled: true, Addr: "127.0.0.1:0"}, msgbus.NewMsgBus())
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if srv == nil {
		t.Fatal("expected server")
	}
	defer srv.Close()

	// Exercise the handler directly (the listener port is not exposed by
	// http.Server when Addr uses :0, so use the mux via a recorder).
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/gc", nil)
	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /gc: status %d, want 405", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/gc", nil)
	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /gc: status %d, want 200", rec.Code)
	}
}

func TestMetricsServerDisabled(t *testing.T) {
	srv, err := StartMetricsServer(MetricsConfig{}, nil)
	if err != nil || srv != nil {
		t.Fatalf("disabled config: srv=%v err=%v, want nil/nil", srv, err)
	}
}

func TestRuntimeConfigGCOffRequiresMemLimit(t *testing.T) {
	// No limit configured and none inherited: refuse.
	prev := debug.SetMemoryLimit(-1)
	if prev != math.MaxInt64 {
		defer debug.SetMemoryLimit(prev)
		debug.SetMemoryLimit(math.MaxInt64)
	}
	err := RuntimeConfig{GCOff: true}.Apply()
	if err == nil {
		t.Fatal("gc_off without memory limit must be refused")
	}

	// With a limit it applies and disables GC; restore both afterwards.
	prevGC := debug.SetGCPercent(100)
	defer debug.SetGCPercent(prevGC)
	defer debug.SetMemoryLimit(math.MaxInt64)
	if err := (RuntimeConfig{GCOff: true, MemLimitBytes: 4 << 30}).Apply(); err != nil {
		t.Fatalf("gc_off with mem limit: %v", err)
	}
	if got := debug.SetMemoryLimit(-1); got != 4<<30 {
		t.Fatalf("memory limit = %d, want %d", got, int64(4<<30))
	}
}
