// Package mobile is the Device Host: the same Go core as the Server Host,
// bound into the Android and iOS apps by gomobile. Only Start and Stop are
// exported; providers, sessions and audio all stay behind the WebSocket.
//
// contract: gomobile binds these two functions only. Add nothing else
// exported here without adding it to the Flutter channel and its tests.
package mobile

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"

	"github.com/wray-lee/parley"
	"github.com/wray-lee/parley/internal/host"
)

var (
	mu     sync.Mutex
	active *host.Server
)

// Start launches the loopback host and returns
// {"url":"ws://127.0.0.1:<port>/ws","token":"<128-bit hex>"}. configJSON uses
// the Server Host's environment keys, so one config loader and one validator
// serve both hosts. PORT and ACCESS_TOKEN in it are ignored: the Device Host
// always picks a free loopback port and a fresh token per launch.
func Start(configJSON string) (string, error) {
	values, err := parseConfig(configJSON)
	if err != nil {
		return "", err
	}

	mu.Lock()
	defer mu.Unlock()
	if active != nil {
		return "", errors.New("the parley host is already running")
	}
	token, err := newToken()
	if err != nil {
		return "", err
	}

	cfg := host.LoadConfig(func(key string) string { return values[key] })
	cfg.Port = 0
	cfg.AccessToken = token
	server, err := host.New(cfg, parley.Assets, host.Options{Loopback: true, RejectOrigin: true})
	if err != nil {
		// A validation message may name the bad provider; it must never echo
		// an API key, even one the user pasted into the wrong field.
		return "", redactError(err, values)
	}
	if err := server.Listen(); err != nil {
		return "", errors.New("the parley host could not open its loopback port")
	}
	active = server

	out, err := json.Marshal(struct {
		URL   string `json:"url"`
		Token string `json:"token"`
	}{URL: "ws://127.0.0.1:" + strconv.Itoa(server.Port()) + "/ws", Token: token})
	if err != nil {
		Stop()
		return "", errors.New("the parley host could not describe its endpoint")
	}
	return string(out), nil
}

// Stop shuts the host down, closing live WebSocket connections and freeing the
// port. It is idempotent.
func Stop() {
	mu.Lock()
	server := active
	active = nil
	mu.Unlock()
	if server != nil {
		_ = server.Close()
	}
}

func parseConfig(configJSON string) (map[string]string, error) {
	values := map[string]string{}
	if strings.TrimSpace(configJSON) == "" {
		return values, nil
	}
	if err := json.Unmarshal([]byte(configJSON), &values); err != nil {
		// Never wrap: the decoder's message can quote the input, which may
		// contain API keys.
		return nil, errors.New("configJSON must be a JSON object of strings")
	}
	return values, nil
}

func newToken() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", errors.New("the parley host could not create its access token")
	}
	return hex.EncodeToString(bytes), nil
}

func redactError(err error, values map[string]string) error {
	message := err.Error()
	for key, value := range values {
		if value == "" {
			continue
		}
		if strings.Contains(key, "KEY") || strings.Contains(key, "TOKEN") {
			message = strings.ReplaceAll(message, value, "<redacted>")
		}
	}
	return errors.New(message)
}
