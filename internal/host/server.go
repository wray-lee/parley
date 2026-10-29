// Server Host: static web client, /healthz, /samples and the WebSocket
// protocol bridge into the Session core. The browser never runs the core.

package host

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/wray-lee/parley/internal/core"
	"github.com/wray-lee/parley/internal/protocol"
	"github.com/wray-lee/parley/internal/providers"
)

// Options are the Server's output sinks. Nil sinks are silent.
type Options struct {
	SegmentLog func(protocol.SegmentLog)
	Warn       func(string)
}

// Server is the Server Host.
type Server struct {
	cfg               Config
	assets            fs.FS
	registry          *core.Registry
	pairConfig        map[string]core.PairConfig
	forcedInstruction string
	sample            providers.Sample
	presets           *providers.Presets
	segmentLog        func(protocol.SegmentLog)
	warn              func(string)
	httpServer        *http.Server
	listener          net.Listener
}

// New loads and validates the product assets and the Provider selection.
func New(cfg Config, assets fs.FS, options Options) (*Server, error) {
	server := &Server{cfg: cfg, assets: assets, segmentLog: options.SegmentLog, warn: options.Warn}
	if server.segmentLog == nil {
		server.segmentLog = func(protocol.SegmentLog) {}
	}
	if server.warn == nil {
		server.warn = func(string) {}
	}
	if server.cfg.AccessToken == "" {
		server.warn("warning: ACCESS_TOKEN is not set; anyone who finds this URL can spend your API quota")
	}

	registry, err := core.LoadRegistry(assets, server.warn)
	if err != nil {
		return nil, err
	}
	server.registry = registry

	server.pairConfig = map[string]core.PairConfig{}
	if data, err := fs.ReadFile(assets, "config/pairs.json"); err != nil {
		server.warn("config/pairs.json ignored: " + err.Error())
	} else if err := json.Unmarshal(data, &server.pairConfig); err != nil {
		server.warn("config/pairs.json ignored: " + err.Error())
		server.pairConfig = map[string]core.PairConfig{}
	}

	if data, err := fs.ReadFile(assets, "prompts/forced_translation.txt"); err != nil {
		server.warn("prompts/forced_translation.txt ignored: " + err.Error())
	} else {
		server.forcedInstruction = strings.TrimSpace(string(data))
	}

	sample, err := providers.LoadSample(assets, "placeholder")
	if err != nil {
		return nil, err
	}
	server.sample = sample

	presets, err := providers.LoadPresets(assets)
	if err != nil {
		return nil, err
	}
	server.presets = presets

	// Like the JavaScript host, validate the ASR/TTS names at startup but
	// build the translator per connection: bad LLM_* config is then a
	// connection error event, not a crash before /healthz answers.
	if _, err := providers.SelectASR(server.cfg.ASRProvider, server.providerConfig().ASROptions, server.sample); err != nil {
		return nil, err
	}
	if _, err := providers.SelectTTS(server.cfg.TTSProvider); err != nil {
		return nil, err
	}
	return server, nil
}

// providerConfig is the Provider selection for every new Session; startup
// validation reuses its ASR options.
func (s *Server) providerConfig() providers.Config {
	return providers.Config{
		ASR: s.cfg.ASRProvider, Translator: s.cfg.TranslatorProvider, TTS: s.cfg.TTSProvider,
		Sample: s.sample, Presets: s.presets,
		ASROptions: providers.ASROptions{
			WhisperLiveKitURL: s.cfg.ASROptions.WhisperLiveKitURL,
			DeepgramAPIKey:    s.cfg.ASROptions.DeepgramAPIKey,
			DashscopeAPIKey:   s.cfg.ASROptions.DashscopeAPIKey,
		},
		LLM: providers.TranslatorOptions{
			Preset:    s.cfg.LLM.Preset,
			BaseURL:   s.cfg.LLM.BaseURL,
			APIKey:    s.cfg.LLM.APIKey,
			Model:     s.cfg.LLM.Model,
			ExtraBody: s.cfg.LLM.ExtraBody,
		},
	}
}

// Listen binds the configured port (0 means any free port) and serves.
func (s *Server) Listen() error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", s.cfg.Port))
	if err != nil {
		return err
	}
	s.listener = listener
	s.httpServer = &http.Server{Handler: s}
	go func() { _ = s.httpServer.Serve(listener) }()
	return nil
}

// Port is the bound port; useful when Config.Port is 0.
func (s *Server) Port() int {
	if s.listener == nil {
		return s.cfg.Port
	}
	return s.listener.Addr().(*net.TCPAddr).Port
}

// Close stops the HTTP server and every open Session.
func (s *Server) Close() error {
	if s.httpServer == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}

// ServeHTTP routes /healthz, /ws and the embedded static client.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path == "/healthz" {
		w.Header().Set("content-type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write([]byte(`{"ok":true}`))
		}
		return
	}
	if r.URL.Path == "/ws" && strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		s.serveWebSocket(w, r)
		return
	}
	s.serveStatic(w, r)
}

func (s *Server) serveStatic(w http.ResponseWriter, r *http.Request) {
	name, ok := staticName(r.URL.Path)
	if !ok {
		writeNotFound(w, r)
		return
	}
	data, err := fs.ReadFile(s.assets, name)
	if err != nil {
		writeNotFound(w, r)
		return
	}
	w.Header().Set("content-type", mimeType(name))
	w.Header().Set("cache-control", "no-store")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
}

func writeNotFound(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	if r.Method != http.MethodHead {
		_, _ = w.Write([]byte("not found"))
	}
}

func staticName(urlPath string) (string, bool) {
	base, relative := "web", strings.TrimPrefix(urlPath, "/")
	if strings.HasPrefix(urlPath, "/samples/") {
		base, relative = "samples", strings.TrimPrefix(urlPath, "/samples/")
	}
	// The Bun host strips every leading slash, so "//index.html" still serves.
	relative = strings.TrimLeft(relative, "/")
	if relative == "" {
		relative = "index.html"
	}
	clean := path.Clean(relative)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return "", false
	}
	return base + "/" + clean, true
}

func mimeType(name string) string {
	switch path.Ext(name) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".wav":
		return "audio/wav"
	case ".mp3":
		return "audio/mpeg"
	}
	return "application/octet-stream"
}

func (s *Server) serveWebSocket(w http.ResponseWriter, r *http.Request) {
	if s.cfg.AccessToken != "" && r.URL.Query().Get("token") != s.cfg.AccessToken {
		body := "access token required"
		w.Header().Set("content-type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		if r.Method != http.MethodHead {
			_, _ = w.Write([]byte(body))
		}
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()

	inputs := make(chan func(), 16)
	posts := make(chan func(), 16)
	closed := make(chan struct{})
	var closeOnce sync.Once
	markClosed := func() { closeOnce.Do(func() { close(closed) }) }

	emit := func(event protocol.Event) {
		writeCtx, cancelWrite := context.WithTimeout(ctx, 10*time.Second)
		defer cancelWrite()
		if chunk, ok := event.(*protocol.TTSChunkEvent); ok {
			_ = conn.Write(writeCtx, websocket.MessageBinary, chunk.Data)
			return
		}
		data, err := json.Marshal(event)
		if err != nil {
			return
		}
		_ = conn.Write(writeCtx, websocket.MessageText, data)
	}
	post := func(callback func()) {
		select {
		case posts <- callback:
		case <-closed:
		case <-ctx.Done():
		}
	}

	asr, translator, tts, err := providers.Select(s.providerConfig())
	if err != nil {
		emit(&protocol.ErrorEvent{Type: "error", Kind: "protocol", Message: "could not start the Session: " + err.Error()})
		_ = conn.Close(websocket.StatusInternalError, "session start failed")
		return
	}
	session := core.NewSession(core.SessionConfig{
		Providers:         core.Providers{ASR: asr, Translator: translator, TTS: tts},
		Clock:             wallClock{post: post},
		Registry:          s.registry,
		PairConfig:        s.pairConfig,
		ForcedInstruction: s.forcedInstruction,
		Emit:              emit,
		Log:               s.segmentLog,
	})

	hello := session.Hello()
	// The Session core does not know the Host's Preset configuration.
	if presetTranslator, ok := translator.(interface{ Preset() providers.Preset }); ok {
		preset := presetTranslator.Preset()
		hello.PresetsInUse = []protocol.Preset{{ID: preset.ID, TrainsOnData: preset.TrainsOnData}}
	}
	payload, err := json.Marshal(hello)
	if err != nil {
		return
	}
	writeCtx, cancelWrite := context.WithTimeout(ctx, 10*time.Second)
	err = conn.Write(writeCtx, websocket.MessageText, payload)
	cancelWrite()
	if err != nil {
		return
	}

	go func() {
		defer markClosed()
		for {
			messageType, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var job func()
			if messageType == websocket.MessageBinary {
				pcm, err := pcm16(data)
				if err != nil {
					job = func() {
						emit(&protocol.ErrorEvent{Type: "error", Kind: "protocol", Message: "invalid audio frame"})
					}
				} else {
					job = func() { session.HandleAudio(pcm) }
				}
			} else {
				var message protocol.ClientMessage
				if err := json.Unmarshal(data, &message); err != nil {
					job = func() {
						emit(&protocol.ErrorEvent{Type: "error", Kind: "protocol", Message: "invalid JSON message"})
					}
				} else {
					job = func() { session.HandleMessage(message) }
				}
			}
			select {
			case inputs <- job:
			case <-closed:
				return
			}
		}
	}()

	for {
		select {
		case job := <-inputs:
			job()
			session.Idle()
		case job := <-posts:
			job()
			session.Idle()
		case <-closed:
			session.Close()
			return
		case <-ctx.Done():
			session.Close()
			return
		}
	}
}

func pcm16(data []byte) ([]int16, error) {
	if len(data)%2 != 0 {
		return nil, fmt.Errorf("odd audio frame length %d", len(data))
	}
	pcm := make([]int16, len(data)/2)
	for i := range pcm {
		pcm[i] = int16(binary.LittleEndian.Uint16(data[i*2:]))
	}
	return pcm, nil
}

type wallClock struct{ post func(func()) }

func (c wallClock) Now() int64 { return time.Now().UnixMilli() }

func (c wallClock) AfterFunc(ms int64, callback func()) core.Timer {
	return &wallTimer{timer: time.AfterFunc(time.Duration(ms)*time.Millisecond, func() { c.post(callback) })}
}

type wallTimer struct{ timer *time.Timer }

func (t *wallTimer) Stop() { t.timer.Stop() }
