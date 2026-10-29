// Shared plumbing for the streaming ASR adapters: one WebSocket connection per
// Open, a generation guard so a stale socket cannot touch the new Session, an
// event buffer drained by Push, and the handshake-status mapping that the
// browser WebSocket API does not expose.
package providers

import (
	"context"
	"encoding/binary"
	"errors"
	"net/http"
	"strconv"
	"sync"

	"github.com/coder/websocket"

	"github.com/wray-lee/parley/internal/core"
	"github.com/wray-lee/parley/internal/protocol"
)

// wsSocket is the WebSocket subset the ASR adapters use. Tests inject a fake.
type wsSocket interface {
	Read(ctx context.Context) (websocket.MessageType, []byte, error)
	Write(ctx context.Context, typ websocket.MessageType, data []byte) error
	CloseNow() error
}

// wsDial dials one WebSocket. status is the HTTP handshake status when the
// server rejected the upgrade, or 0.
type wsDial func(ctx context.Context, url string, header http.Header, subprotocols []string) (wsSocket, int, error)

func dialWebSocket(ctx context.Context, url string, header http.Header, subprotocols []string) (wsSocket, int, error) {
	conn, response, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPHeader:   header,
		Subprotocols: subprotocols,
	})
	status := 0
	if response != nil {
		status = response.StatusCode
	}
	if err != nil {
		return nil, status, err
	}
	return conn, status, nil
}

// asrError is the Go form of the JavaScript adapters' frozen error event:
// {type: "error", provider: "asr", kind, message, fallback}.
func asrError(kind, message, fallback string) error {
	return &protocol.ProviderError{Kind: kind, Message: message, Fallback: fallback}
}

func pcmBytes(pcm []int16) []byte {
	if len(pcm) == 0 {
		return nil
	}
	data := make([]byte, len(pcm)*2)
	for index, sample := range pcm {
		binary.LittleEndian.PutUint16(data[index*2:], uint16(sample))
	}
	return data
}

func closeCode(err error) string {
	var closed websocket.CloseError
	if errors.As(err, &closed) {
		return strconv.Itoa(int(closed.Code))
	}
	return "unknown"
}

// socketASR is embedded by each adapter; all access goes through mu.
type socketASR struct {
	mu           sync.Mutex
	ctx          context.Context
	cancel       context.CancelFunc
	conn         wsSocket
	generation   int
	opened       bool
	closing      bool
	failed       bool
	events       []core.ASREvent
	failure      error
	pendingAudio [][]byte
	dial         wsDial
}

func newSocketASR(dial wsDial) socketASR {
	if dial == nil {
		dial = dialWebSocket
	}
	return socketASR{dial: dial}
}

// begin discards the previous connection and resets the per-Session state.
// The returned generation is the only one allowed to deliver anything.
func (s *socketASR) begin() (context.Context, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
	if s.conn != nil {
		_ = s.conn.CloseNow()
	}
	s.conn = nil
	s.generation++
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.opened = false
	s.closing = false
	s.failed = false
	s.events = nil
	s.failure = nil
	s.pendingAudio = nil
	return s.ctx, s.generation
}

// abortPendingDialLocked cancels a handshake that Close interrupted. The
// adapter context has no deadline, so without this the dial (and its TCP
// attempt) would outlive the Session; the oracle's close() aborts a
// CONNECTING WebSocket the same way.
func (s *socketASR) abortPendingDialLocked() {
	if s.conn == nil && s.cancel != nil {
		s.cancel()
	}
}

// failLocked records one hard failure; the adapter stops using the socket.
func (s *socketASR) failLocked(err error) {
	if err == nil || s.failed || s.closing {
		return
	}
	s.failed = true
	s.raiseLocked(err)
}

// raiseLocked records a soft error (a malformed or in-band error frame) but
// keeps the socket usable, like the JavaScript adapters' deliver().
func (s *socketASR) raiseLocked(err error) {
	if err != nil && s.failure == nil {
		s.failure = err
	}
}

func (s *socketASR) deliverLocked(events ...core.ASREvent) {
	s.events = append(s.events, events...)
}

// take returns the buffered events and at most one error. A hard failure is
// reported once; later Pushes stay quiet until the next Open.
func (s *socketASR) take() ([]core.ASREvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	events := s.events
	s.events = nil
	err := s.failure
	s.failure = nil
	return events, err
}

// write sends one frame outside the lock so a slow write cannot stall the
// reader. A send failure is a hard adapter failure, as in JavaScript.
func (s *socketASR) write(mine int, typ websocket.MessageType, data []byte, onFail func(error) error) {
	s.mu.Lock()
	conn, closing, ctx := s.conn, s.closing, s.ctx
	s.mu.Unlock()
	if conn == nil || closing {
		return
	}
	if err := conn.Write(ctx, typ, data); err != nil {
		s.mu.Lock()
		if mine == s.generation {
			s.failLocked(onFail(err))
		}
		s.mu.Unlock()
	}
}

// run dials, flushes queued audio, then reads until the connection ends. All
// state changes are generation-guarded so a replaced socket is inert.
func (s *socketASR) run(
	ctx context.Context,
	mine int,
	target string,
	header http.Header,
	subprotocols []string,
	openFrame []byte,
	flushOnOpen bool,
	handleLocked func([]byte),
	dialError func(status int, err error) error,
	closedError func(code string) error,
	sendError func(error) error,
) {
	conn, status, err := s.dial(ctx, target, header, subprotocols)
	if err != nil {
		s.mu.Lock()
		if mine == s.generation {
			s.failLocked(dialError(status, err))
		}
		s.mu.Unlock()
		return
	}
	s.mu.Lock()
	if mine != s.generation {
		s.mu.Unlock()
		_ = conn.CloseNow()
		return
	}
	s.conn = conn
	s.opened = true
	closedWhileDialing := s.closing
	pending := s.pendingAudio
	if flushOnOpen {
		s.pendingAudio = nil
	}
	s.mu.Unlock()

	if closedWhileDialing {
		_ = conn.CloseNow()
		return
	}
	if openFrame != nil {
		if err := conn.Write(ctx, websocket.MessageText, openFrame); err != nil {
			s.mu.Lock()
			if mine == s.generation {
				s.failLocked(sendError(err))
			}
			s.mu.Unlock()
			return
		}
	}
	if flushOnOpen {
		for _, chunk := range pending {
			s.write(mine, websocket.MessageBinary, chunk, sendError)
		}
	}
	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			s.mu.Lock()
			if mine == s.generation {
				s.failLocked(closedError(closeCode(err)))
			}
			s.mu.Unlock()
			return
		}
		if typ != websocket.MessageText {
			continue
		}
		s.mu.Lock()
		if mine == s.generation {
			handleLocked(data)
		}
		s.mu.Unlock()
	}
}

// pushAudio sends one microphone frame when ready, otherwise queues it.
// Closing drops frames, exactly like the JavaScript adapters.
func (s *socketASR) pushAudio(pcm []int16, ready func() bool, sendError func(error) error) {
	data := pcmBytes(pcm)
	if len(data) == 0 {
		return
	}
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return
	}
	if ready() && s.conn != nil {
		conn, ctx, mine := s.conn, s.ctx, s.generation
		s.mu.Unlock()
		if err := conn.Write(ctx, websocket.MessageBinary, data); err != nil {
			s.mu.Lock()
			if mine == s.generation {
				s.failLocked(sendError(err))
			}
			s.mu.Unlock()
		}
		return
	}
	s.pendingAudio = append(s.pendingAudio, data)
	s.mu.Unlock()
}
