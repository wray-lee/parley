package host_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/wray-lee/parley/internal/protocol"
)

// ASR_PROVIDER=deepgram must be wired end to end: a missing key surfaces as a
// protocol error event on the first audio frame, without any network call.
func TestDeepgramASRErrorReachesTheClient(t *testing.T) {
	_, port, _ := startServer(t, map[string]string{"ASR_PROVIDER": "deepgram"})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, fmt.Sprintf("ws://127.0.0.1:%d/ws", port), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.CloseNow()

	var hello protocol.Hello
	readJSON(t, ctx, conn, &hello)
	writeJSON(t, ctx, conn, protocol.ClientMessage{Type: "start", Mode: "listen", Src: "zh", Tgt: "ja"})
	writePCM(t, ctx, conn, 6000)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		messageType, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if messageType != websocket.MessageText {
			continue
		}
		var event struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(data, &event); err != nil {
			t.Fatalf("event: %v", err)
		}
		if event.Type != "error" {
			continue
		}
		var failure protocol.ErrorEvent
		if err := json.Unmarshal(data, &failure); err != nil {
			t.Fatal(err)
		}
		if failure.Provider != "asr" || failure.Kind != "auth" ||
			!strings.Contains(failure.Fallback, "DEEPGRAM_API_KEY") {
			t.Fatalf("error event = %+v", failure)
		}
		return
	}
	t.Fatal("no ASR error event")
}
