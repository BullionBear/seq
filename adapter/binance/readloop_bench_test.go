package binance

import (
	"net"
	"net/http"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/BullionBear/seq/core/msgbus"
	"github.com/lxzan/gws"
)

// P2-1 verification: measure the full frame-read -> parse -> publish path
// over a loopback WebSocket connection, including the gws read loop.
//
// gws reads each frame into a buffer taken from its internal sharded pool;
// message.Close() in OnMessage recycles it, so the payload buffer itself is
// allocation-free in steady state.
//
// Measured result: 2 allocs/op, 40 B/op. Memory profiling attributes them to
// (1) gws readMessage — the *gws.Message envelope the library constructs
// before dispatching to OnMessage, the only per-frame allocation on the
// production read path — and (2) gws WriteMessage on the benchmark's own
// server-side sender, which does not exist in production. Everything on our
// side of OnMessage is zero-alloc (see TestProcessMessageZeroAllocs).

// benchServerHandler is the pumping side; it never receives data frames.
type benchServerHandler struct{ gws.BuiltinEventHandler }

// benchClientHandler mirrors wsEventHandler minus the deadline bookkeeping:
// it runs the production parse+publish path and counts processed frames.
type benchClientHandler struct {
	client *BinanceSpotDataClient
	done   atomic.Int64
}

func (h *benchClientHandler) OnOpen(*gws.Conn)         {}
func (h *benchClientHandler) OnClose(*gws.Conn, error) {}
func (h *benchClientHandler) OnPing(*gws.Conn, []byte) {}
func (h *benchClientHandler) OnPong(*gws.Conn, []byte) {}

func (h *benchClientHandler) OnMessage(_ *gws.Conn, message *gws.Message) {
	h.client.processMessage(message.Bytes())
	_ = message.Close()
	h.done.Add(1)
}

func BenchmarkFrameReadParsePublish(b *testing.B) {
	client, eb := newAllocTestClient(b)

	// Loopback server that upgrades one connection and hands it back.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	serverConns := make(chan *gws.Conn, 1)
	upgrader := gws.NewUpgrader(&benchServerHandler{}, &gws.ServerOption{
		CheckUtf8Enabled: false,
	})
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		socket, err := upgrader.Upgrade(w, r)
		if err != nil {
			return
		}
		serverConns <- socket
		socket.ReadLoop()
	})}
	go func() { _ = srv.Serve(listener) }()
	defer srv.Close()

	handler := &benchClientHandler{client: client}
	clientConn, _, err := gws.NewClient(handler, &gws.ClientOption{
		Addr:             "ws://" + listener.Addr().String(),
		ReadBufferSize:   wsReadBufferSize,
		CheckUtf8Enabled: false,
	})
	if err != nil {
		b.Fatalf("dial: %v", err)
	}
	go clientConn.ReadLoop()
	serverConn := <-serverConns

	// Drain the event bus continuously so the arena/ring never overflow.
	stop := make(chan struct{})
	drained := make(chan struct{})
	drain := func(msgbus.Event) {}
	go func() {
		defer close(drained)
		for {
			for eb.Poll(drain) {
			}
			select {
			case <-stop:
				return
			default:
				runtime.Gosched()
			}
		}
	}()

	// Warm up pools, scratch buffers, and caches.
	for i := 0; i < 16; i++ {
		if err := serverConn.WriteMessage(gws.OpcodeText, binanceCombinedDepthMsg); err != nil {
			b.Fatalf("warmup write: %v", err)
		}
	}
	for handler.done.Load() < 16 {
		runtime.Gosched()
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := serverConn.WriteMessage(gws.OpcodeText, binanceCombinedDepthMsg); err != nil {
			b.Fatalf("write: %v", err)
		}
	}
	for handler.done.Load() < int64(b.N)+16 {
		runtime.Gosched()
	}
	b.StopTimer()

	close(stop)
	<-drained
	_ = clientConn.WriteClose(1000, nil)
}
