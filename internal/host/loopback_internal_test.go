package host

import (
	"net"
	"testing"

	"github.com/wray-lee/parley"
)

func TestLoopbackListenBinds127001(t *testing.T) {
	server, err := New(LoadConfig(func(key string) string {
		if key == "PORT" {
			return "0"
		}
		return ""
	}), parley.Assets, Options{Loopback: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := server.Listen(); err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer server.Close()

	addr, ok := server.listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener address %v is not TCP", server.listener.Addr())
	}
	if !addr.IP.IsLoopback() || addr.IP.String() != "127.0.0.1" {
		t.Fatalf("bound address = %v, want 127.0.0.1", addr)
	}
}
