// Server Host: static web client, /healthz, /samples and the WebSocket
// protocol bridge into the Session core. The browser never runs the core.

package host

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
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
	// RejectOrigin makes every request that carries an Origin header fail, as
	// the Device Host's Loopback Host must (ADR-0003). The Server Host leaves
	// it false because browsers connect with an Origin.
	RejectOrigin bool
	// Loopback binds 127.0.0.1 instead of every interface, as the Device Host's
	// Loopback Host must (ADR-0003). The Server Host leaves it false.
	Loopback bool
	// Host overrides the bind address (HOST=127.0.0.1 keeps a local check off
	// the firewall). Empty binds every interface. Loopback wins when both set.
	Host string
	// Web is the Flutter web release build to serve at /. Nil keeps the
	// embedded vanilla client at / and still exposes it at /lite.
	Web fs.FS
}

// Server is the Server Host.
type Server struct {
	cfg               Config
	assets            fs.FS
	web               fs.FS
	registry          *core.Registry
	pairConfig        map[string]core.PairConfig
	forcedInstruction string
	sample            providers.Sample
	samples           *providers.SampleBook
	presets           *providers.Presets
	voices            providers.Voices
	segmentLog        func(protocol.SegmentLog)
	warn              func(string)
	rejectOrigin      bool
	loopback          bool
	bindHost          string
	httpServer        *http.Server
	listener          net.Listener

	connMu sync.Mutex
	conns  map[*websocket.Conn]struct{}
}

// New loads and validates the product assets and the Provider selection.
func New(cfg Config, assets fs.FS, options Options) (*Server, error) {
	server := &Server{cfg: cfg, assets: assets, web: options.Web, segmentLog: options.SegmentLog, warn: options.Warn, rejectOrigin: options.RejectOrigin, loopback: options.Loopback, bindHost: options.Host, conns: map[*websocket.Conn]struct{}{}}
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

	sampleBook, err := providers.LoadSampleBook(assets)
	if err != nil {
		return nil, err
	}
	server.samples = sampleBook
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

	voices, err := providers.LoadVoices(assets)
	if err != nil {
		return nil, err
	}
	server.voices = voices

	// Like the JavaScript host, validate the ASR/TTS names at startup but
	// build the translator per connection: bad LLM_* config is then a
	// connection error event, not a crash before /healthz answers.
	providerConfig := server.providerConfig(server.samples)
	if _, err := providers.SelectASR(server.cfg.ASRProvider, providerConfig.ASROptions, server.sample); err != nil {
		return nil, err
	}
	if _, err := providers.SelectTTS(server.cfg.TTSProvider, providerConfig.TTSOptions, server.voices); err != nil {
		return nil, err
	}
	return server, nil
}

// providerConfig is the Provider selection for a new Session; startup
// validation reuses its ASR options. samples is that Session's own copy of
// the SampleBook, so `sample {name}` never reaches another connection.
func (s *Server) providerConfig(samples *providers.SampleBook) providers.Config {
	return providers.Config{
		ASR: s.cfg.ASRProvider, Translator: s.cfg.TranslatorProvider, TTS: s.cfg.TTSProvider,
		Sample: s.sample, Samples: samples, Presets: s.presets,
		ASROptions: providers.ASROptions{
			WhisperLiveKitURL: s.cfg.ASROptions.WhisperLiveKitURL,
			DeepgramAPIKey:    s.cfg.ASROptions.DeepgramAPIKey,
			DashscopeAPIKey:   s.cfg.ASROptions.DashscopeAPIKey,
		},
		TTSOptions: providers.TTSOptions{
			KokoroURL: s.cfg.TTSOptions.KokoroURL, AzureKey: s.cfg.TTSOptions.AzureKey,
			AzureRegion: s.cfg.TTSOptions.AzureRegion, Warn: s.warn,
		},
		Voices: s.voices,
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
	host := s.bindHost
	if s.loopback {
		host = "127.0.0.1"
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(host, fmt.Sprint(s.cfg.Port)))
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

// Close stops the HTTP server and every open Session. WebSocket connections
// are hijacked, so Shutdown does not close them: close them explicitly or the
// next launch could not reuse the port.
func (s *Server) Close() error {
	if s.httpServer == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := s.httpServer.Shutdown(ctx)

	s.connMu.Lock()
	conns := make([]*websocket.Conn, 0, len(s.conns))
	for conn := range s.conns {
		conns = append(conns, conn)
	}
	s.connMu.Unlock()
	for _, conn := range conns {
		conn.CloseNow()
	}
	return err
}

// ServeHTTP routes /healthz, /ws and the embedded static client.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.rejectOrigin && r.Header.Get("Origin") != "" {
		w.WriteHeader(http.StatusForbidden)
		return
	}
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

// staticCSP is strict enough to keep the app same-origin: the Flutter loader
// needs wasm compilation, inline styles and its same-origin WebSocket, nothing
// else. fonts.gstatic.com is unreachable by design.
const staticCSP = "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; " +
	"style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; " +
	"font-src 'self'; connect-src 'self'; media-src 'self' blob:; " +
	"worker-src 'self' blob:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'"

func (s *Server) serveStatic(w http.ResponseWriter, r *http.Request) {
	var fsys fs.FS
	base, relative := "web", r.URL.Path
	usingWeb := false
	switch {
	case r.URL.Path == "/lite":
		// Relative assets would resolve at the root without the slash.
		http.Redirect(w, r, "/lite/", http.StatusMovedPermanently)
		return
	case strings.HasPrefix(r.URL.Path, "/lite/"):
		relative = strings.TrimPrefix(r.URL.Path, "/lite/")
	case strings.HasPrefix(r.URL.Path, "/samples/"):
		base, relative = "samples", strings.TrimPrefix(r.URL.Path, "/samples/")
	default:
		if s.web != nil {
			fsys, base, usingWeb = s.web, "", true
		}
	}
	if fsys == nil {
		fsys = s.assets
	}
	name, ok := staticName(base, relative)
	if !ok {
		writeNotFound(w, r)
		return
	}
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		// The vanilla client keeps two root-absolute asset URLs (/main.js,
		// /mic-worklet.js) from its root-serving days; redirect them to the
		// /lite mount so its relative module imports resolve under /lite/.
		if usingWeb && s.redirectVanilla(w, r) {
			return
		}
		writeNotFound(w, r)
		return
	}
	writeStatic(w, r, name, data)
}

func (s *Server) redirectVanilla(w http.ResponseWriter, r *http.Request) bool {
	name, ok := staticName("web", r.URL.Path)
	if !ok {
		return false
	}
	if _, err := fs.Stat(s.assets, name); err != nil {
		return false
	}
	http.Redirect(w, r, "/lite/"+strings.TrimLeft(r.URL.Path, "/"), http.StatusMovedPermanently)
	return true
}

func writeNotFound(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	if r.Method != http.MethodHead {
		_, _ = w.Write([]byte("not found"))
	}
}

func staticName(base, relative string) (string, bool) {
	// The Bun host strips every leading slash, so "//index.html" still serves.
	relative = strings.TrimLeft(relative, "/")
	if relative == "" {
		relative = "index.html"
	}
	clean := path.Clean(relative)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return "", false
	}
	if base == "" {
		return clean, true
	}
	return base + "/" + clean, true
}

func writeStatic(w http.ResponseWriter, r *http.Request, name string, data []byte) {
	w.Header().Set("content-security-policy", staticCSP)
	w.Header().Set("x-content-type-options", "nosniff")
	w.Header().Set("referrer-policy", "no-referrer")
	w.Header().Set("content-type", mimeType(name))
	w.Header().Set("cache-control", "no-store")
	// CanvasKit, main.dart.js and the entry HTML dominate the cold load, so
	// compress them; fonts and images are already compressed.
	if len(data) >= 1024 && compressible(name) && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("content-encoding", "gzip")
		w.Header().Add("vary", "Accept-Encoding")
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodHead {
			return
		}
		writer := gzip.NewWriter(w)
		_, _ = io.Copy(writer, bytes.NewReader(data))
		_ = writer.Close()
		return
	}
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
}

func compressible(name string) bool {
	switch path.Ext(name) {
	case ".html", ".js", ".mjs", ".wasm", ".css", ".json", ".map", ".svg", ".txt":
		return true
	}
	return false
}

func mimeType(name string) string {
	switch path.Ext(name) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".json", ".map":
		return "application/json; charset=utf-8"
	case ".wasm":
		return "application/wasm"
	case ".woff2":
		return "font/woff2"
	case ".ttf":
		return "font/ttf"
	case ".otf":
		return "font/otf"
	case ".png":
		return "image/png"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	case ".txt":
		return "text/plain; charset=utf-8"
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
	// The Bun oracle accepts every browser Origin: ACCESS_TOKEN is the guard
	// and tunnels/CDNs may rewrite Host, so the same-origin default would break
	// deploys. The Loopback Host rejects every Origin above (ADR-0003).
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	s.connMu.Lock()
	s.conns[conn] = struct{}{}
	s.connMu.Unlock()
	defer func() {
		s.connMu.Lock()
		delete(s.conns, conn)
		s.connMu.Unlock()
	}()

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

	samples := s.samples.Session()
	asr, translator, tts, err := providers.Select(s.providerConfig(samples))
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
					job = func() {
						if message.Type == "sample" && !samples.Select(message.Name) {
							emit(&protocol.ErrorEvent{
								Type:    "error",
								Kind:    "protocol",
								Message: fmt.Sprintf("unknown sample %q", message.Name),
							})
						}
						session.HandleMessage(message)
					}
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
