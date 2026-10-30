// Package conformance is the black-box Conformance Suite (Seam 3). Every case
// speaks only the WebSocket protocol, so the same cases run against the Bun
// Server Host (the frozen oracle), the Go Server Host and, later, the Device
// Host's loopback URL. See spec/conformance/README.md and spec/DIVERGENCES.md.
//
// Run against a host that is already running:
//
//	go test ./spec/conformance -url ws://127.0.0.1:3000/ws
//	go test ./spec/conformance -url ws://127.0.0.1:<port>/ws?token=<token> -origin reject
//
// Without -url the tests start the Go host in process and compare it with the
// Bun host that is still the oracle until P4.
package conformance

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/wray-lee/parley"
	"github.com/wray-lee/parley/internal/host"
)

var (
	flagURL    = flag.String("url", "", "run the Conformance Suite against this ws(s) URL and exit")
	flagOrigin = flag.String("origin", "allow", "Origin policy of the target host: allow (Server Host) or reject (Device Host)")
)

const (
	dialTimeout    = 10 * time.Second
	messageTimeout = 10 * time.Second
	audioFrameMS   = 20
	// gatingReleaseWait is longer than the core's 500 ms Gating tail.
	gatingReleaseWait = 700 * time.Millisecond
)

type options struct {
	// originReject is the Device Host's Loopback Host policy (ADR-0003): any
	// request carrying an Origin header is refused. Server Hosts accept them.
	originReject bool
}

type result struct {
	name   string
	status string // pass, fail or skip
	detail string
}

type skipError struct{ reason string }

func (e skipError) Error() string { return e.reason }

type conformanceCase struct {
	name string
	run  func(rawURL string, opts options) error
}

// runSuite executes every case on its own connection and reports one result
// per case. A failing case never stops the others: the comparison between
// hosts needs the full picture.
func runSuite(rawURL string, opts options) []result {
	cases := []conformanceCase{
		{"hello", caseHello},
		{"start listen", caseStartListen},
		{"demo segment", caseDemoSegment},
		{"segment ordering", caseSegmentOrdering},
		{"commit immutability", caseCommitImmutability},
		{"tts ordering", caseTTSOrdering},
		{"unknown direction error", caseUnknownDirection},
		{"invalid JSON error", caseInvalidJSON},
		{"converse gating", caseConverseGating},
		{"listen gating delta", caseListenGatingDelta},
		{"hold merge", caseHoldMerge},
		{"hold forced", caseHoldForced},
		{"token rejection", caseTokenRejection},
		{"origin policy", caseOriginPolicy},
	}
	results := make([]result, 0, len(cases))
	for _, candidate := range cases {
		err := candidate.run(rawURL, opts)
		var skip skipError
		switch {
		case err == nil:
			results = append(results, result{name: candidate.name, status: "pass"})
		case errors.As(err, &skip):
			results = append(results, result{name: candidate.name, status: "skip", detail: skip.reason})
		default:
			results = append(results, result{name: candidate.name, status: "fail", detail: err.Error()})
		}
	}
	return results
}

// --- connection helper -----------------------------------------------------

// client is one WebSocket connection that records every message so cases can
// look backwards (ordering) as well as wait forwards.
type client struct {
	conn     *websocket.Conn
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	messages []map[string]any
	binaries []int // number of JSON messages seen when a binary frame arrived
	err      error
	wake     chan struct{}
}

func dial(rawURL string, header http.Header) (*client, error) {
	ctx, cancel := context.WithCancel(context.Background())
	dialCtx, dialCancel := context.WithTimeout(ctx, dialTimeout)
	defer dialCancel()
	conn, _, err := websocket.Dial(dialCtx, rawURL, &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		cancel()
		return nil, err
	}
	c := &client{conn: conn, ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1)}
	go c.read()
	return c, nil
}

func (c *client) read() {
	for {
		messageType, data, err := c.conn.Read(c.ctx)
		if err != nil {
			c.mu.Lock()
			c.err = err
			c.mu.Unlock()
			c.notify()
			return
		}
		c.mu.Lock()
		if messageType == websocket.MessageBinary {
			c.binaries = append(c.binaries, len(c.messages))
		} else {
			var message map[string]any
			if json.Unmarshal(data, &message) == nil {
				c.messages = append(c.messages, message)
			}
		}
		c.mu.Unlock()
		c.notify()
	}
}

func (c *client) notify() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *client) close() {
	c.cancel()
	_ = c.conn.CloseNow()
}

func (c *client) send(message any) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return c.conn.Write(c.ctx, websocket.MessageText, data)
}

func (c *client) sendText(text string) error {
	return c.conn.Write(c.ctx, websocket.MessageText, []byte(text))
}

func (c *client) sendFrame(amplitude float64) error {
	return c.conn.Write(c.ctx, websocket.MessageBinary, pcmFrame(amplitude))
}

func (c *client) sendAudio(amplitude float64, frames int) error {
	for index := 0; index < frames; index++ {
		if err := c.sendFrame(amplitude); err != nil {
			return err
		}
	}
	return nil
}

func (c *client) waitFor(label string, timeout time.Duration, match func(map[string]any) bool) (map[string]any, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		c.mu.Lock()
		for _, message := range c.messages {
			if match(message) {
				c.mu.Unlock()
				return message, nil
			}
		}
		readErr := c.err
		c.mu.Unlock()
		if readErr != nil {
			return nil, fmt.Errorf("socket closed while waiting for %s: %w", label, readErr)
		}
		select {
		case <-c.wake:
		case <-timer.C:
			return nil, fmt.Errorf("timed out waiting for %s", label)
		case <-c.ctx.Done():
			return nil, c.ctx.Err()
		}
	}
}

func (c *client) count(match func(map[string]any) bool) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	total := 0
	for _, message := range c.messages {
		if match(message) {
			total++
		}
	}
	return total
}

func (c *client) indexOf(match func(map[string]any) bool) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	for index, message := range c.messages {
		if match(message) {
			return index
		}
	}
	return -1
}

func (c *client) firstBinaryPosition() (int, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.binaries) == 0 {
		return 0, false
	}
	return c.binaries[0], true
}

// handshakeStatus dials once and returns the HTTP status, closing the socket.
func handshakeStatus(rawURL string, header http.Header) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()
	conn, response, err := websocket.Dial(ctx, rawURL, &websocket.DialOptions{HTTPHeader: header})
	if conn != nil {
		conn.CloseNow()
	}
	if response != nil {
		if response.Body != nil {
			response.Body.Close()
		}
		return response.StatusCode, nil
	}
	return 0, err
}

// --- message helpers -------------------------------------------------------

func typeOf(message map[string]any) string {
	value, _ := message["type"].(string)
	return value
}

func stringField(message map[string]any, key string) string {
	value, _ := message[key].(string)
	return value
}

func intField(message map[string]any, key string) (int, bool) {
	value, ok := message[key].(float64)
	return int(value), ok
}

func directionOf(message map[string]any) (string, string) {
	direction, _ := message["direction"].(map[string]any)
	return stringField(direction, "src"), stringField(direction, "tgt")
}

func isType(want string) func(map[string]any) bool {
	return func(message map[string]any) bool { return typeOf(message) == want }
}

func isASRError(message map[string]any) bool {
	return typeOf(message) == "error" && stringField(message, "provider") == "asr"
}

func pcmFrame(amplitude float64) []byte {
	const samples = 16000 * audioFrameMS / 1000
	pcm := make([]int16, samples)
	for index := range pcm {
		pcm[index] = int16(math.Round(math.Sin(2*math.Pi*220*float64(index)/16000) * amplitude * 32767))
	}
	data := make([]byte, len(pcm)*2)
	for index, sample := range pcm {
		binary.LittleEndian.PutUint16(data[index*2:], uint16(sample))
	}
	return data
}

// --- cases -----------------------------------------------------------------

func caseHello(rawURL string, _ options) error {
	conn, err := dial(rawURL, nil)
	if err != nil {
		return err
	}
	defer conn.close()
	hello, err := conn.waitFor("hello", messageTimeout, isType("hello"))
	if err != nil {
		return err
	}
	if demo, _ := hello["demo"].(bool); !demo {
		return errors.New("conformance expects Demo Mode (mock Providers)")
	}
	pairs, _ := hello["pairs"].([]any)
	converse := false
	for _, candidate := range pairs {
		pair, _ := candidate.(map[string]any)
		a, b := stringField(pair, "a"), stringField(pair, "b")
		if (a == "ja" && b == "zh") || (a == "zh" && b == "ja") {
			converse, _ = pair["converse"].(bool)
		}
	}
	if !converse {
		return errors.New("hello must expose the converse-capable ja/zh pair")
	}
	if _, ok := hello["packs"].([]any); !ok {
		return errors.New("hello must list packs")
	}
	// Additive v0.1 fields: tolerated only with the values DIVERGENCES lists.
	if value, ok := hello["protocol"]; ok {
		if protocol, _ := value.(float64); protocol != 1 {
			return fmt.Errorf("hello.protocol = %v, want 1", value)
		}
	}
	if value, ok := hello["tts"]; ok {
		tts, _ := value.(string)
		if tts != "system" && tts != "audio" {
			return fmt.Errorf("hello.tts = %q, want system or audio", tts)
		}
	}
	// Auto Turn is additive: absent, or {available, enabled} booleans. With the
	// Demo mock Provider nothing is validated, so v0.1 must report Manual.
	if value, ok := hello["autoTurn"]; ok {
		auto, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("hello.autoTurn = %v, want an object", value)
		}
		available, okAvailable := auto["available"].(bool)
		enabled, okEnabled := auto["enabled"].(bool)
		if !okAvailable || !okEnabled {
			return fmt.Errorf("hello.autoTurn = %v, want boolean available and enabled", value)
		}
		if available || enabled {
			return fmt.Errorf("hello.autoTurn = %v, want Manual for the mock Provider", value)
		}
	}
	return nil
}

// startListen opens a Listen Session and returns the connection at the
// listening state. The caller closes it.
func startListen(conn *client) error {
	if _, err := conn.waitFor("hello", messageTimeout, isType("hello")); err != nil {
		return err
	}
	if err := conn.send(map[string]any{"type": "start", "mode": "listen", "src": "zh", "tgt": "ja"}); err != nil {
		return err
	}
	state, err := conn.waitFor("state listening", messageTimeout, isType("state"))
	if err != nil {
		return err
	}
	if state["state"] != "listening" {
		return fmt.Errorf("unexpected state %v", state["state"])
	}
	return nil
}

// driveDemo replays the bundled sample: 1.6 s of speech, then 0.5 s of
// silence so the VAD Cut fires.
func driveDemo(conn *client) error {
	if err := conn.send(map[string]any{"type": "sample"}); err != nil {
		return err
	}
	if err := conn.sendAudio(0.2, 80); err != nil {
		return err
	}
	return conn.sendAudio(0, 25)
}

// playDemo drives the bundled sample into one committed Segment.
func playDemo(conn *client) (map[string]any, error) {
	if err := driveDemo(conn); err != nil {
		return nil, err
	}
	return conn.waitFor("segment", messageTimeout, isType("segment"))
}

func checkDemoSegment(segment map[string]any) error {
	segID, ok := intField(segment, "seg_id")
	if !ok || segID <= 0 {
		return fmt.Errorf("seg_id must be a positive integer, got %v", segment["seg_id"])
	}
	if stringField(segment, "src") == "" || stringField(segment, "tgt") == "" {
		return errors.New("segment.src and segment.tgt must be non-empty")
	}
	src, tgt := directionOf(segment)
	if src != "zh" || tgt != "ja" {
		return fmt.Errorf("direction = %s->%s, want zh->ja", src, tgt)
	}
	if demo, _ := segment["demo"].(bool); !demo {
		return errors.New("a Demo Segment must be DEMO-labelled")
	}
	if speak, _ := segment["speak"].(bool); !speak {
		return errors.New("Listen must speak its Direction")
	}
	return nil
}

func caseStartListen(rawURL string, _ options) error {
	conn, err := dial(rawURL, nil)
	if err != nil {
		return err
	}
	defer conn.close()
	return startListen(conn)
}

func caseDemoSegment(rawURL string, _ options) error {
	conn, err := dial(rawURL, nil)
	if err != nil {
		return err
	}
	defer conn.close()
	if err := startListen(conn); err != nil {
		return err
	}
	segment, err := playDemo(conn)
	if err != nil {
		return err
	}
	return checkDemoSegment(segment)
}

func caseSegmentOrdering(rawURL string, _ options) error {
	conn, err := dial(rawURL, nil)
	if err != nil {
		return err
	}
	defer conn.close()
	if err := startListen(conn); err != nil {
		return err
	}
	first, err := playDemo(conn)
	if err != nil {
		return err
	}
	firstID, ok := intField(first, "seg_id")
	if !ok {
		return errors.New("first segment has no seg_id")
	}
	// The sample replays on the same Session, so seg_id must keep increasing.
	if err := driveDemo(conn); err != nil {
		return err
	}
	second, err := conn.waitFor("second segment", messageTimeout, func(message map[string]any) bool {
		segID, _ := intField(message, "seg_id")
		return typeOf(message) == "segment" && segID != firstID
	})
	if err != nil {
		return err
	}
	secondID, _ := intField(second, "seg_id")
	if secondID <= firstID {
		return fmt.Errorf("seg_id went %d -> %d; committed Segments must be ordered", firstID, secondID)
	}
	return checkDemoSegment(second)
}

func caseCommitImmutability(rawURL string, _ options) error {
	conn, err := dial(rawURL, nil)
	if err != nil {
		return err
	}
	defer conn.close()
	if err := startListen(conn); err != nil {
		return err
	}
	segment, err := playDemo(conn)
	if err != nil {
		return err
	}
	segID, _ := intField(segment, "seg_id")
	// Give a revising implementation time to send its revision.
	time.Sleep(500 * time.Millisecond)
	copies := conn.count(func(message map[string]any) bool {
		id, _ := intField(message, "seg_id")
		return typeOf(message) == "segment" && id == segID
	})
	if copies != 1 {
		return fmt.Errorf("committed seg_id %d appeared %d times; committed output is never revised", segID, copies)
	}
	return nil
}

func caseTTSOrdering(rawURL string, _ options) error {
	conn, err := dial(rawURL, nil)
	if err != nil {
		return err
	}
	defer conn.close()
	if err := startListen(conn); err != nil {
		return err
	}
	segment, err := playDemo(conn)
	if err != nil {
		return err
	}
	segID, _ := intField(segment, "seg_id")
	if _, err := conn.waitFor("tts_end", messageTimeout, func(message map[string]any) bool {
		id, _ := intField(message, "seg_id")
		return typeOf(message) == "tts_end" && id == segID
	}); err != nil {
		return err
	}
	begin := conn.indexOf(func(message map[string]any) bool {
		id, _ := intField(message, "seg_id")
		return typeOf(message) == "tts_begin" && id == segID
	})
	end := conn.indexOf(func(message map[string]any) bool {
		id, _ := intField(message, "seg_id")
		return typeOf(message) == "tts_end" && id == segID
	})
	binaryPosition, ok := conn.firstBinaryPosition()
	if begin < 0 || end < 0 || !ok {
		return errors.New("tts_begin, a binary TTS chunk and tts_end must all be sent")
	}
	if !(begin < binaryPosition && binaryPosition <= end) {
		return fmt.Errorf("TTS order violated: tts_begin at %d, binary at %d, tts_end at %d", begin, binaryPosition, end)
	}
	return nil
}

func caseUnknownDirection(rawURL string, _ options) error {
	conn, err := dial(rawURL, nil)
	if err != nil {
		return err
	}
	defer conn.close()
	if _, err := conn.waitFor("hello", messageTimeout, isType("hello")); err != nil {
		return err
	}
	if err := conn.send(map[string]any{"type": "start", "mode": "listen", "src": "zh", "tgt": "xx"}); err != nil {
		return err
	}
	failure, err := conn.waitFor("protocol error", messageTimeout, isType("error"))
	if err != nil {
		return err
	}
	if kind := stringField(failure, "kind"); kind != "protocol" {
		return fmt.Errorf("unexpected error kind %q", kind)
	}
	if message := stringField(failure, "message"); !strings.Contains(message, "unknown direction") {
		return fmt.Errorf("error message %q does not name the unknown direction", message)
	}
	return nil
}

func caseInvalidJSON(rawURL string, _ options) error {
	conn, err := dial(rawURL, nil)
	if err != nil {
		return err
	}
	defer conn.close()
	if _, err := conn.waitFor("hello", messageTimeout, isType("hello")); err != nil {
		return err
	}
	if err := conn.sendText("not json"); err != nil {
		return err
	}
	failure, err := conn.waitFor("protocol error", messageTimeout, isType("error"))
	if err != nil {
		return err
	}
	if kind := stringField(failure, "kind"); kind != "protocol" {
		return fmt.Errorf("unexpected error kind %q", kind)
	}
	return nil
}

// startConverse opens a Converse Session and plays the sample until the first
// Segment's TTS has ended, so a playback entry is registered. It returns the
// connection and the Segment; the caller closes the connection.
func startConverse(rawURL string) (*client, map[string]any, error) {
	conn, err := dial(rawURL, nil)
	if err != nil {
		return nil, nil, err
	}
	if _, err := conn.waitFor("hello", messageTimeout, isType("hello")); err != nil {
		conn.close()
		return nil, nil, err
	}
	if err := conn.send(map[string]any{"type": "start", "mode": "converse", "src": "zh", "tgt": "ja"}); err != nil {
		conn.close()
		return nil, nil, err
	}
	if _, err := conn.waitFor("state listening", messageTimeout, isType("state")); err != nil {
		conn.close()
		return nil, nil, err
	}
	segment, err := playDemo(conn)
	if err != nil {
		conn.close()
		return nil, nil, err
	}
	segID, _ := intField(segment, "seg_id")
	if _, err := conn.waitFor("tts_end", messageTimeout, func(message map[string]any) bool {
		id, _ := intField(message, "seg_id")
		return typeOf(message) == "tts_end" && id == segID
	}); err != nil {
		conn.close()
		return nil, nil, err
	}
	return conn, segment, nil
}

func caseConverseGating(rawURL string, _ options) error {
	conn, segment, err := startConverse(rawURL)
	if err != nil {
		return err
	}
	defer conn.close()
	segID, _ := intField(segment, "seg_id")

	// The client reports it is on the live microphone. The Demo ASR refuses
	// live audio, so its error event is the sentinel for "the frame reached ASR".
	if err := conn.send(map[string]any{"type": "input", "source": "live"}); err != nil {
		return err
	}
	if err := conn.sendFrame(0.2); err != nil {
		return err
	}
	time.Sleep(300 * time.Millisecond)
	if conn.count(isASRError) != 0 {
		return errors.New("the microphone reached ASR while the app was speaking; Converse must Gate")
	}

	if err := conn.send(map[string]any{"type": "playback", "seg_id": segID, "state": "end"}); err != nil {
		return err
	}
	time.Sleep(gatingReleaseWait)
	if err := conn.sendFrame(0.2); err != nil {
		return err
	}
	if _, err := conn.waitFor("the mock microphone error after playback", messageTimeout, isASRError); err != nil {
		return fmt.Errorf("the Gate did not release after playback ended: %w", err)
	}
	return nil
}

// caseListenGatingDelta encodes the gating-scope row of spec/DIVERGENCES.md:
// the v0.0.3 oracle gates Listen too, while the v0.1 core never gates Listen.
// Both outcomes pass; the hello.protocol capability selects the expectation.
func caseListenGatingDelta(rawURL string, _ options) error {
	conn, err := dial(rawURL, nil)
	if err != nil {
		return err
	}
	defer conn.close()
	hello, err := conn.waitFor("hello", messageTimeout, isType("hello"))
	if err != nil {
		return err
	}
	_, v01 := hello["protocol"]
	if err := startListen(conn); err != nil {
		return err
	}
	segment, err := playDemo(conn)
	if err != nil {
		return err
	}
	segID, _ := intField(segment, "seg_id")
	if _, err := conn.waitFor("tts_end", messageTimeout, func(message map[string]any) bool {
		id, _ := intField(message, "seg_id")
		return typeOf(message) == "tts_end" && id == segID
	}); err != nil {
		return err
	}
	if err := conn.send(map[string]any{"type": "input", "source": "live"}); err != nil {
		return err
	}
	if err := conn.sendFrame(0.2); err != nil {
		return err
	}
	if v01 {
		// v0.1: Listen never gates, so the frame reaches the mock ASR at once.
		if _, err := conn.waitFor("the mock microphone error", messageTimeout, isASRError); err != nil {
			return fmt.Errorf("v0.1 Listen must not Gate: %w", err)
		}
		return nil
	}
	// v0.0.3 oracle: the playback still withholds the frame.
	time.Sleep(300 * time.Millisecond)
	if conn.count(isASRError) != 0 {
		return errors.New("the v0.0.3 oracle must withhold Listen microphone frames while speaking")
	}
	if err := conn.send(map[string]any{"type": "playback", "seg_id": segID, "state": "end"}); err != nil {
		return err
	}
	time.Sleep(gatingReleaseWait)
	if err := conn.sendFrame(0.2); err != nil {
		return err
	}
	if _, err := conn.waitFor("the mock microphone error after playback", messageTimeout, isASRError); err != nil {
		return fmt.Errorf("the v0.0.3 Gate did not release: %w", err)
	}
	return nil
}

// playHeldSample drives the ja-held sample (first line translates to ⏸) up to
// the first Cut. It returns the held event, or a skip when the host's
// translator cannot produce the hold mark.
func playHeldSample(conn *client) (map[string]any, error) {
	if err := conn.send(map[string]any{"type": "start", "mode": "listen", "src": "ja", "tgt": "zh"}); err != nil {
		return nil, err
	}
	if _, err := conn.waitFor("state listening", messageTimeout, isType("state")); err != nil {
		return nil, err
	}
	if err := conn.send(map[string]any{"type": "sample", "name": "ja-held"}); err != nil {
		return nil, err
	}
	// 2.6 s of speech covers the first transcript line, then silence Cuts.
	if err := conn.sendAudio(0.2, 130); err != nil {
		return nil, err
	}
	if err := conn.sendAudio(0, 20); err != nil {
		return nil, err
	}
	got, err := conn.waitFor("hold or segment", messageTimeout, func(message map[string]any) bool {
		return typeOf(message) == "hold" || typeOf(message) == "segment"
	})
	if err != nil {
		return nil, err
	}
	if typeOf(got) == "segment" {
		return nil, skipError{"the host translates the held line as a Segment; Demo Mode's mock translator prefixes [DEMO] and cannot answer the ⏸ hold mark (Hold is proven by spec/traces/listen-hold-*)"}
	}
	if held, _ := got["held"].(bool); !held {
		return nil, errors.New("hold event must say held: true")
	}
	return got, nil
}

func caseHoldMerge(rawURL string, _ options) error {
	conn, err := dial(rawURL, nil)
	if err != nil {
		return err
	}
	defer conn.close()
	if _, err := conn.waitFor("hello", messageTimeout, isType("hello")); err != nil {
		return err
	}
	if _, err := playHeldSample(conn); err != nil {
		return err
	}
	// The second transcript line lets the Cut merge into the Held Segment.
	if err := conn.sendAudio(0.2, 90); err != nil {
		return err
	}
	if err := conn.sendAudio(0, 20); err != nil {
		return err
	}
	segment, err := conn.waitFor("merged segment", messageTimeout, isType("segment"))
	if err != nil {
		return err
	}
	src, tgt := stringField(segment, "src"), stringField(segment, "tgt")
	if !strings.Contains(src, "確認が終わってから") || !strings.Contains(src, "提出されます") {
		return fmt.Errorf("merged Segment src = %q", src)
	}
	if tgt == "" || tgt == "⏸" {
		return fmt.Errorf("merged Segment tgt = %q; the Hold must be retried with the merged text", tgt)
	}
	return nil
}

func caseHoldForced(rawURL string, _ options) error {
	conn, err := dial(rawURL, nil)
	if err != nil {
		return err
	}
	defer conn.close()
	if _, err := conn.waitFor("hello", messageTimeout, isType("hello")); err != nil {
		return err
	}
	if _, err := playHeldSample(conn); err != nil {
		return err
	}
	// No second line: after 2.5 s the core forces the Held Segment. The
	// translator still answers ⏸, so the Forced Translation commits the source.
	segment, err := conn.waitFor("forced segment", 8*time.Second, isType("segment"))
	if err != nil {
		return err
	}
	if stringField(segment, "tgt") != "" {
		return fmt.Errorf("a Forced Translation that still answers ⏸ must commit the source only; tgt = %q", stringField(segment, "tgt"))
	}
	if src := stringField(segment, "src"); !strings.Contains(src, "確認が終わってから") {
		return fmt.Errorf("forced Segment src = %q", src)
	}
	return nil
}

func caseTokenRejection(rawURL string, _ options) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	token := parsed.Query().Get("token")
	if token == "" {
		return skipError{"the host has no ACCESS_TOKEN (no ?token= in -url)"}
	}
	missing := *parsed
	missing.RawQuery = ""
	if status, err := handshakeStatus(missing.String(), nil); err != nil {
		return fmt.Errorf("missing token: %w", err)
	} else if status != http.StatusUnauthorized {
		return fmt.Errorf("missing token handshake = %d, want 401", status)
	}
	wrong := *parsed
	query := wrong.Query()
	query.Set("token", token+"-wrong")
	wrong.RawQuery = query.Encode()
	if status, err := handshakeStatus(wrong.String(), nil); err != nil {
		return fmt.Errorf("wrong token: %w", err)
	} else if status != http.StatusUnauthorized {
		return fmt.Errorf("wrong token handshake = %d, want 401", status)
	}
	return nil
}

func caseOriginPolicy(rawURL string, opts options) error {
	origin := "https://evil.example"
	if !opts.originReject {
		// A Server Host serves its web client same-origin; Caddy passes Host
		// through, so the browser's Origin equals the socket's host.
		parsed, err := url.Parse(rawURL)
		if err != nil {
			return err
		}
		scheme := "http"
		if parsed.Scheme == "wss" || parsed.Scheme == "https" {
			scheme = "https"
		}
		origin = scheme + "://" + parsed.Host
	}
	status, err := handshakeStatus(rawURL, http.Header{"Origin": []string{origin}})
	if err != nil {
		return err
	}
	if opts.originReject {
		if status != http.StatusForbidden {
			return fmt.Errorf("Origin-bearing connection = %d, want 403", status)
		}
		return nil
	}
	if status != http.StatusSwitchingProtocols {
		return fmt.Errorf("the same-origin web client was refused; handshake = %d", status)
	}
	return nil
}

// --- command line ----------------------------------------------------------

func TestMain(m *testing.M) {
	flag.Parse()
	if *flagURL != "" {
		opts := options{}
		switch *flagOrigin {
		case "allow":
		case "reject":
			opts.originReject = true
		default:
			fmt.Fprintf(os.Stderr, "-origin must be allow or reject, got %q\n", *flagOrigin)
			os.Exit(2)
		}
		exit := 0
		for _, outcome := range runSuite(*flagURL, opts) {
			switch outcome.status {
			case "pass":
				fmt.Printf("ok - %s\n", outcome.name)
			case "skip":
				fmt.Printf("skip - %s (%s)\n", outcome.name, outcome.detail)
			default:
				fmt.Printf("FAIL - %s: %s\n", outcome.name, outcome.detail)
				exit = 1
			}
		}
		os.Exit(exit)
	}
	os.Exit(m.Run())
}

// --- tests -----------------------------------------------------------------

func startGoHost(t *testing.T, values map[string]string, opts host.Options) *host.Server {
	t.Helper()
	values["PORT"] = "0"
	server, err := host.New(host.LoadConfig(func(key string) string { return values[key] }), parley.Assets, opts)
	if err != nil {
		t.Fatalf("host.New: %v", err)
	}
	if err := server.Listen(); err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })
	return server
}

func resultsByName(results []result) map[string]result {
	byName := make(map[string]result, len(results))
	for _, outcome := range results {
		byName[outcome.name] = outcome
	}
	return byName
}

func logResults(t *testing.T, label string, results []result) {
	for _, outcome := range results {
		t.Logf("%s: %s - %s (%s)", label, outcome.status, outcome.name, outcome.detail)
	}
}

func assertNoFailures(t *testing.T, label string, results []result) {
	t.Helper()
	for _, outcome := range results {
		if outcome.status == "fail" {
			t.Errorf("%s: %s: %s", label, outcome.name, outcome.detail)
		}
	}
}

func TestConformanceAgainstGoHost(t *testing.T) {
	server := startGoHost(t, map[string]string{}, host.Options{})
	rawURL := fmt.Sprintf("ws://127.0.0.1:%d/ws", server.Port())
	results := runSuite(rawURL, options{})
	logResults(t, "go", results)
	assertNoFailures(t, "go", results)
	for _, outcome := range results {
		if outcome.name == "hello" || outcome.name == "demo segment" || outcome.name == "converse gating" {
			if outcome.status != "pass" {
				t.Errorf("%s = %s (%s)", outcome.name, outcome.status, outcome.detail)
			}
		}
	}
}

// TestConformanceHoldWithFakeTranslator proves Hold and Forced behaviour over
// the wire. Demo Mode cannot: its mock translator prefixes [DEMO] to the ⏸
// mark. A fake OpenAI-compatible endpoint supplies the mark instead.
func TestConformanceHoldWithFakeTranslator(t *testing.T) {
	translator := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		text := ""
		for _, message := range request.Messages {
			text += message.Content
		}
		answer := "译"
		if strings.Contains(text, "確認が終わってから") && !strings.Contains(text, "提出されます") {
			answer = "⏸"
		}
		w.Header().Set("content-type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\ndata: [DONE]\n\n", answer)
	}))
	t.Cleanup(translator.Close)

	server := startGoHost(t, map[string]string{
		"LLM_BASE_URL": translator.URL + "/v1",
		"LLM_API_KEY":  "fake-key-not-real",
	}, host.Options{})
	rawURL := fmt.Sprintf("ws://127.0.0.1:%d/ws", server.Port())
	results := resultsByName(runSuite(rawURL, options{}))
	for _, name := range []string{"hold merge", "hold forced"} {
		outcome := results[name]
		if outcome.status != "pass" {
			t.Errorf("%s = %s (%s)", name, outcome.status, outcome.detail)
		}
	}
}

func TestConformanceTokenAndOriginRejectionOnLoopbackHost(t *testing.T) {
	const token = "fake-token-not-real"
	server := startGoHost(t, map[string]string{"ACCESS_TOKEN": token}, host.Options{RejectOrigin: true})
	rawURL := fmt.Sprintf("ws://127.0.0.1:%d/ws?token=%s", server.Port(), token)
	results := runSuite(rawURL, options{originReject: true})
	logResults(t, "loopback", results)
	assertNoFailures(t, "loopback", results)
	byName := resultsByName(results)
	for _, name := range []string{"token rejection", "origin policy"} {
		outcome := byName[name]
		if outcome.status != "pass" {
			t.Errorf("%s = %s (%s)", name, outcome.status, outcome.detail)
		}
	}
}

// TestConformanceBunAndGoIdentical requires the same results after ignoring
// only the deltas spec/DIVERGENCES.md lists. The cases themselves encode the
// gating-scope delta; -origin allow is the Server Host contract.
func TestConformanceBunAndGoIdentical(t *testing.T) {
	bunPath, err := exec.LookPath("bun")
	if err != nil {
		t.Skip("bun is not installed; the Bun oracle comparison needs Bun 1.4.3")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "node_modules", "ws")); err != nil {
		t.Skip("node_modules/ws is missing; run bun install first")
	}

	const token = "fake-token-not-real"
	goServer := startGoHost(t, map[string]string{"ACCESS_TOKEN": token}, host.Options{})
	goURL := fmt.Sprintf("ws://127.0.0.1:%d/ws?token=%s", goServer.Port(), token)

	port := freePort(t)
	bunURL := fmt.Sprintf("ws://127.0.0.1:%d/ws?token=%s", port, token)
	startBunHost(t, bunPath, root, port, token)

	goSuite := runSuite(goURL, options{})
	bunSuite := runSuite(bunURL, options{})
	logResults(t, "go", goSuite)
	logResults(t, "bun", bunSuite)
	goResults := resultsByName(goSuite)
	bunResults := resultsByName(bunSuite)

	if len(goResults) != len(bunResults) {
		t.Fatalf("Go reported %d cases, Bun reported %d", len(goResults), len(bunResults))
	}
	for name, goOutcome := range goResults {
		bunOutcome, ok := bunResults[name]
		if !ok {
			t.Errorf("Bun did not report case %q", name)
			continue
		}
		if goOutcome.status != bunOutcome.status {
			t.Errorf("%q: Go %s (%s), Bun %s (%s)", name, goOutcome.status, goOutcome.detail, bunOutcome.status, bunOutcome.detail)
		}
	}
}

// TestConformanceFailsOnDeliberateProtocolRegression mutates one field of the
// hello message in transit. The suite must notice.
func TestConformanceFailsOnDeliberateProtocolRegression(t *testing.T) {
	server := startGoHost(t, map[string]string{}, host.Options{})
	upstream := fmt.Sprintf("ws://127.0.0.1:%d/ws", server.Port())
	rawURL := mangleHelloProxy(t, upstream)
	results := resultsByName(runSuite(rawURL, options{}))
	hello, ok := results["hello"]
	if !ok {
		t.Fatal("the suite did not run the hello case")
	}
	if hello.status != "fail" {
		t.Fatalf("hello = %s; a deliberately regressed hello must fail the suite", hello.status)
	}
}

// mangleHelloProxy relays WebSocket frames and flips hello.demo to false.
func mangleHelloProxy(t *testing.T, upstream string) string {
	t.Helper()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		downstream, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer downstream.CloseNow()
		up, _, err := websocket.Dial(ctx, upstream, nil)
		if err != nil {
			_ = downstream.Close(websocket.StatusInternalError, "upstream unreachable")
			return
		}
		defer up.CloseNow()

		go func() {
			for {
				messageType, data, err := up.Read(ctx)
				if err != nil {
					_ = downstream.Close(websocket.StatusNormalClosure, "")
					return
				}
				if messageType == websocket.MessageText {
					var message map[string]any
					if json.Unmarshal(data, &message) == nil && message["type"] == "hello" {
						message["demo"] = false
						data, _ = json.Marshal(message)
					}
				}
				if err := downstream.Write(ctx, messageType, data); err != nil {
					return
				}
			}
		}()
		for {
			messageType, data, err := downstream.Read(ctx)
			if err != nil {
				return
			}
			if err := up.Write(ctx, messageType, data); err != nil {
				return
			}
		}
	})
	proxy := httptest.NewServer(handler)
	t.Cleanup(proxy.Close)
	return "ws" + strings.TrimPrefix(proxy.URL, "http") + "/ws"
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	return port
}

// startBunHost boots the frozen Bun Server Host in Demo Mode on the given
// port. The environment is scrubbed of every key that could leave Demo Mode.
func startBunHost(t *testing.T, bunPath, root string, port int, token string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	command := exec.CommandContext(ctx, bunPath, "--no-env-file", "hosts/node/server.js")
	command.Dir = root
	command.Env = append(cleanEnvironment(), fmt.Sprintf("PORT=%d", port), "ACCESS_TOKEN="+token)
	output := &lockedBuffer{}
	command.Stdout = output
	command.Stderr = output
	if err := command.Start(); err != nil {
		t.Fatalf("start bun: %v", err)
	}
	t.Cleanup(func() {
		_ = command.Process.Kill()
		_, _ = command.Process.Wait()
	})
	deadline := time.Now().Add(30 * time.Second)
	for !strings.Contains(output.String(), "Parley listening") {
		if time.Now().After(deadline) {
			t.Fatalf("the Bun Server Host did not listen:\n%s", output.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func cleanEnvironment() []string {
	drop := map[string]bool{
		"ACCESS_TOKEN": true, "PORT": true, "PUBLIC_DOMAIN": true,
		"ASR_PROVIDER": true, "TTS_PROVIDER": true,
		"LLM_PRESET": true, "LLM_BASE_URL": true, "LLM_API_KEY": true,
		"LLM_MODEL": true, "LLM_EXTRA_BODY": true,
		"WHISPERLIVEKIT_URL": true, "DEEPGRAM_API_KEY": true, "DASHSCOPE_API_KEY": true,
		"KOKORO_URL": true, "AZURE_SPEECH_KEY": true, "AZURE_SPEECH_REGION": true,
	}
	env := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !drop[key] {
			env = append(env, entry)
		}
	}
	return env
}

type lockedBuffer struct {
	mu   sync.Mutex
	data []byte
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.data)
}
