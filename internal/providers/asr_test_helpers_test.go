package providers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/wray-lee/parley/internal/core"
	"github.com/wray-lee/parley/internal/protocol"
)

// fakeSocket is the transport double: Write records frames, Read blocks until
// the socket is discarded. Tests feed inbound frames through receive().
type fakeSocket struct {
	mu      sync.Mutex
	writes  []fakeWrite
	closed  bool
	closeCh chan struct{}
	once    sync.Once
}

type fakeWrite struct {
	typ  websocket.MessageType
	data []byte
}

var errHandshake = errors.New("handshake rejected")

func newFakeSocket() *fakeSocket {
	return &fakeSocket{closeCh: make(chan struct{})}
}

func (f *fakeSocket) Read(ctx context.Context) (websocket.MessageType, []byte, error) {
	select {
	case <-ctx.Done():
		return 0, nil, ctx.Err()
	case <-f.closeCh:
		return 0, nil, errors.New("socket closed")
	}
}

func (f *fakeSocket) Write(_ context.Context, typ websocket.MessageType, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return errors.New("socket closed")
	}
	f.writes = append(f.writes, fakeWrite{typ: typ, data: append([]byte(nil), data...)})
	return nil
}

func (f *fakeSocket) CloseNow() error {
	f.once.Do(func() {
		f.mu.Lock()
		f.closed = true
		f.mu.Unlock()
		close(f.closeCh)
	})
	return nil
}

func (f *fakeSocket) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

func (f *fakeSocket) sent() []fakeWrite {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeWrite(nil), f.writes...)
}

// fakeTransport dials fake sockets and records the handshake exactly once per
// attempt. A hold channel delays the handshake so queueing can be observed.
type fakeTransport struct {
	mu           sync.Mutex
	status       int
	err          error
	hold         chan struct{}
	calls        int
	returned     int
	urls         []string
	headers      []http.Header
	subprotocols [][]string
	sockets      []*fakeSocket
}

func (f *fakeTransport) dial(ctx context.Context, url string, header http.Header, subprotocols []string) (wsSocket, int, error) {
	f.mu.Lock()
	f.calls++
	f.urls = append(f.urls, url)
	f.headers = append(f.headers, header.Clone())
	f.subprotocols = append(f.subprotocols, append([]string(nil), subprotocols...))
	hold, status, err := f.hold, f.status, f.err
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.returned++
		f.mu.Unlock()
	}()
	if hold != nil {
		select {
		case <-hold:
		case <-ctx.Done():
			return nil, status, ctx.Err()
		}
	}
	if err != nil {
		return nil, status, err
	}
	socket := newFakeSocket()
	f.mu.Lock()
	f.sockets = append(f.sockets, socket)
	f.mu.Unlock()
	return socket, 0, nil
}

func (f *fakeTransport) attempt(t *testing.T) (string, http.Header, []string) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls == 0 {
		t.Fatal("no dial attempt")
	}
	return f.urls[0], f.headers[0], f.subprotocols[0]
}

func (f *fakeTransport) socket(t *testing.T, index int) *fakeSocket {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if index < 0 {
		index += len(f.sockets)
	}
	if index < 0 || index >= len(f.sockets) {
		t.Fatalf("socket %d of %d", index, len(f.sockets))
	}
	return f.sockets[index]
}

func waitSocket(t *testing.T, transport *fakeTransport, index int) *fakeSocket {
	t.Helper()
	waitFor(t, "the dialed socket", func() bool {
		transport.mu.Lock()
		defer transport.mu.Unlock()
		return len(transport.sockets) > index
	})
	return transport.socket(t, index)
}

// waitDialReturned proves a held handshake was aborted: only Close can release
// it, since hold is never closed.
func waitDialReturned(t *testing.T, transport *fakeTransport) {
	t.Helper()
	waitFor(t, "the pending dial to abort", func() bool {
		transport.mu.Lock()
		defer transport.mu.Unlock()
		return transport.returned > 0
	})
}

func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func waitOpened(t *testing.T, state *socketASR) {
	t.Helper()
	waitFor(t, "the socket to open", func() bool {
		state.mu.Lock()
		defer state.mu.Unlock()
		return state.opened || state.failure != nil
	})
}

func requireProviderError(t *testing.T, err error, kind, messagePart, fallbackPart string) *protocol.ProviderError {
	t.Helper()
	if err == nil {
		t.Fatal("expected a Provider error")
	}
	var providerError *protocol.ProviderError
	if !errors.As(err, &providerError) {
		t.Fatalf("error is not a ProviderError: %v", err)
	}
	if providerError.Kind != kind {
		t.Fatalf("kind = %q, want %q (%v)", providerError.Kind, kind, providerError)
	}
	if !strings.Contains(providerError.Message, messagePart) {
		t.Fatalf("message %q does not contain %q", providerError.Message, messagePart)
	}
	if fallbackPart != "" && !strings.Contains(providerError.Fallback, fallbackPart) {
		t.Fatalf("fallback %q does not contain %q", providerError.Fallback, fallbackPart)
	}
	return providerError
}

func readFixture(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
}

type eventView struct {
	Type    string
	Text    string
	Glue    *string
	StartMS int64
	EndMS   int64
}

func eventsView(events []core.ASREvent) []eventView {
	views := make([]eventView, len(events))
	for index, event := range events {
		views[index] = eventView{
			Type: event.Type, Text: event.Text, Glue: event.Glue,
			StartMS: event.StartMS, EndMS: event.EndMS,
		}
	}
	return views
}
