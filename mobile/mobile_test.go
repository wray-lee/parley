package mobile_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/wray-lee/parley/internal/protocol"
	"github.com/wray-lee/parley/mobile"
)

type startInfo struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

func start(t *testing.T, configJSON string) startInfo {
	t.Helper()
	raw, err := mobile.Start(configJSON)
	if err != nil {
		t.Fatalf("Start(%q): %v", configJSON, err)
	}
	t.Cleanup(mobile.Stop)
	var info startInfo
	if err := json.Unmarshal([]byte(raw), &info); err != nil {
		t.Fatalf("Start result %q is not JSON: %v", raw, err)
	}
	return info
}

func dial(t *testing.T, rawURL string, header http.Header) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return websocket.Dial(ctx, rawURL, &websocket.DialOptions{HTTPHeader: header})
}

func readHello(t *testing.T, conn *websocket.Conn) protocol.Hello {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read hello: %v", err)
	}
	var hello protocol.Hello
	if err := json.Unmarshal(data, &hello); err != nil {
		t.Fatalf("hello %q is not JSON: %v", data, err)
	}
	return hello
}

func TestStartReturnsLoopbackURLAnd128BitToken(t *testing.T) {
	info := start(t, "")

	parsed, err := neturl(info.URL)
	if err != nil {
		t.Fatalf("url %q: %v", info.URL, err)
	}
	if parsed.host != "127.0.0.1" || parsed.port == "" || parsed.path != "/ws" {
		t.Fatalf("url = %q, want ws://127.0.0.1:<port>/ws", info.URL)
	}
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(info.Token) {
		t.Fatalf("token = %q, want 32 hex chars (128 bits)", info.Token)
	}

	conn, response, err := dial(t, info.URL+"?token="+info.Token, nil)
	if err != nil {
		t.Fatalf("dial: %v (%v)", err, response)
	}
	defer conn.CloseNow()
	readHello(t, conn)
}

func TestStartRejectsASecondHostUntilStop(t *testing.T) {
	start(t, "")
	if _, err := mobile.Start(""); err == nil {
		t.Fatal("second Start succeeded; want an error while a host is running")
	}
	mobile.Stop()
	// Stop is idempotent and a new Start works afterwards.
	mobile.Stop()
	second := start(t, "")
	if second.Token == "" {
		t.Fatal("Start after Stop returned no token")
	}
}

func TestFreshTokenAndPortPerLaunch(t *testing.T) {
	first := start(t, "")
	mobile.Stop()
	second := start(t, "")

	if first.Token == second.Token {
		t.Errorf("token reused across launches: %q", first.Token)
	}
}

func TestStopClosesLiveSocketsAndFreesThePort(t *testing.T) {
	info := start(t, "")
	conn, _, err := dial(t, info.URL+"?token="+info.Token, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.CloseNow()
	readHello(t, conn)

	mobile.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, _, err := conn.Read(ctx); err == nil {
		t.Fatal("read after Stop succeeded; the live socket is still open")
	}

	port := neturlMust(t, info.URL).port
	rebound, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		t.Fatalf("port %s is not free after Stop: %v", port, err)
	}
	_ = rebound.Close()
}

func TestTokenAndOriginGuards(t *testing.T) {
	info := start(t, "")

	if _, response, err := dial(t, info.URL, nil); err == nil {
		t.Fatal("token-less dial succeeded; want 401")
	} else if response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("token-less dial = %v (response %v), want 401", err, response)
	}

	if _, response, err := dial(t, info.URL+"?token=wrong", nil); err == nil {
		t.Fatal("wrong-token dial succeeded; want 401")
	} else if response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong-token dial = %v (response %v), want 401", err, response)
	}

	header := http.Header{"Origin": {"https://example.test"}}
	if _, response, err := dial(t, info.URL+"?token="+info.Token, header); err == nil {
		t.Fatal("Origin-bearing dial succeeded; want 403")
	} else if response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("Origin-bearing dial = %v (response %v), want 403", err, response)
	}
}

// TestConfigJSONUsesServerEnvironmentKeys proves the Device Host and the
// Server Host share one loader: the same keys change the same hello fields.
func TestConfigJSONUsesServerEnvironmentKeys(t *testing.T) {
	info := start(t, `{"TTS_PROVIDER":"system","ASR_PROVIDER":"mock","ACCESS_TOKEN":"ignored","PORT":"1"}`)
	conn, _, err := dial(t, info.URL+"?token="+info.Token, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.CloseNow()
	hello := readHello(t, conn)

	if hello.TTS != "system" {
		t.Errorf("hello.tts = %q, want system from TTS_PROVIDER", hello.TTS)
	}
	if !hello.Demo {
		t.Error("hello.demo = false, want true for mock providers")
	}
	// PORT and ACCESS_TOKEN in the config JSON must not override the loopback
	// port or the per-launch token.
	if neturlMust(t, info.URL).port == "1" {
		t.Error("config JSON PORT overrode the loopback port")
	}
	if info.Token == "ignored" {
		t.Error("config JSON ACCESS_TOKEN overrode the per-launch token")
	}
}

func TestStartErrorsNeverEchoSecretValues(t *testing.T) {
	const secret = "sentinel-secret-value-123"

	_, err := mobile.Start(`{"ASR_PROVIDER":"bogus","LLM_API_KEY":"` + secret + `"}`)
	if err == nil {
		t.Fatal("Start with a bogus ASR_PROVIDER succeeded; want an error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("Start error echoes the API key: %v", err)
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("Start error %q does not name the invalid provider", err)
	}

	_, err = mobile.Start(`{"LLM_API_KEY":`)
	if err == nil {
		t.Fatal("Start with malformed JSON succeeded; want an error")
	}
	if strings.Contains(err.Error(), "LLM_API_KEY") || strings.Contains(err.Error(), secret) {
		t.Fatalf("malformed-config error echoes the input: %v", err)
	}
}

// neturl is a tiny URL splitter so the test does not need net/url for a
// ws URL (net/url would reject an empty host but accepts ws fine; keep the
// assertions explicit).
type wsURL struct {
	host string
	port string
	path string
}

func neturl(raw string) (wsURL, error) {
	trimmed := strings.TrimPrefix(raw, "ws://")
	hostPort, path, _ := strings.Cut(trimmed, "/")
	host, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		return wsURL{}, err
	}
	return wsURL{host: host, port: port, path: "/" + path}, nil
}

func neturlMust(t *testing.T, raw string) wsURL {
	t.Helper()
	parsed, err := neturl(raw)
	if err != nil {
		t.Fatalf("url %q: %v", raw, err)
	}
	return parsed
}
