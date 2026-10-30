package host_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/wray-lee/parley/internal/host"
)

// TestLoopbackHostIsLoopbackOnlyAndClosesLiveSockets covers the Device Host
// contract: 127.0.0.1 only, Origin rejected, and Close closing hijacked
// WebSocket connections so the port is free for the next launch.
func TestLoopbackHostIsLoopbackOnlyAndClosesLiveSockets(t *testing.T) {
	server, port, _ := startServerWith(t,
		map[string]string{"ACCESS_TOKEN": "tok"},
		host.Options{Loopback: true, RejectOrigin: true},
	)
	rawURL := fmt.Sprintf("ws://127.0.0.1:%d/ws?token=tok", port)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, response, err := websocket.Dial(ctx, rawURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": {"https://example.test"}},
	}); err == nil {
		t.Fatal("dial with Origin succeeded; want 403")
	} else if response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("dial with Origin = %v (response %v), want 403", err, response)
	}

	conn, _, err := websocket.Dial(ctx, rawURL, nil)
	if err != nil {
		t.Fatalf("dial loopback host: %v", err)
	}
	defer conn.CloseNow()

	// Consume hello so the read below tests the socket, not buffered data.
	if _, _, err := conn.Read(ctx); err != nil {
		t.Fatalf("read hello: %v", err)
	}

	if err := server.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	readCtx, cancelRead := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelRead()
	if _, _, err := conn.Read(readCtx); err == nil {
		t.Fatal("read after Close succeeded; the live socket is still open")
	}

	rebound, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("port %d is not free after Close: %v", port, err)
	}
	_ = rebound.Close()
}
