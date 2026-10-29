// Microsoft Edge read-aloud: keyless, unofficial and unstable. This is an
// independent Go implementation of the publicly observable WebSocket protocol;
// no code was copied from the LGPL edge-tts project. Ported from
// core/providers/tts-edge.js.
//
// Frames: a JSON speech.config text frame, an SSML text frame, then binary
// frames of [2-byte big-endian header length][header][mp3 payload]; a text
// frame with Path:turn.end closes the response.
//
// shortcut: the Sec-MS-GEC version pin ages with Edge, update it when the
// handshake starts returning 403.

package providers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/wray-lee/parley/internal/protocol"
)

const (
	edgeTrustedClientToken = "6A5AA1D4EAFF4E9FB37E23D68491D6F4"
	edgeEndpoint           = "wss://speech.platform.bing.com/consumer/speech/synthesize/readaloud/edge/v1"
	edgeOutputFormat       = "audio-24khz-48kbitrate-mono-mp3"
	edgeSecGecVersion      = "2-143.0.3650.96"
	edgeCopyright          = "Copyright (C) Microsoft Corporation. All rights reserved."
	edgeFallback           = "Set TTS_PROVIDER=kokoro or system."
	edgeDefaultTimeout     = 30 * time.Second
	edgeUserAgent          = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36 Edg/143.0.0.0"
)

// edgeConn is the WebSocket seam; *websocket.Conn satisfies it.
type edgeConn interface {
	Read(ctx context.Context) (websocket.MessageType, []byte, error)
	Write(ctx context.Context, typ websocket.MessageType, data []byte) error
	Close(code websocket.StatusCode, reason string) error
}

type edgeDial func(ctx context.Context, url string, options *websocket.DialOptions) (edgeConn, *http.Response, error)

func defaultEdgeDial(ctx context.Context, url string, options *websocket.DialOptions) (edgeConn, *http.Response, error) {
	conn, response, err := websocket.Dial(ctx, url, options)
	if conn == nil {
		return nil, response, err
	}
	return conn, response, err
}

// EdgeOptions are the voices data plus test seams.
type EdgeOptions struct {
	Voices  map[string][]string
	Now     func() time.Time
	Timeout time.Duration
	Warn    func(string)
	Dial    edgeDial
}

// EdgeTTS is the Edge read-aloud Provider.
type EdgeTTS struct {
	voices  map[string][]string
	now     func() time.Time
	timeout time.Duration
	warn    func(string)
	dial    edgeDial
}

// NewEdgeTTS builds the adapter from config data.
func NewEdgeTTS(options EdgeOptions) *EdgeTTS {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	timeout := options.Timeout
	if timeout == 0 {
		timeout = edgeDefaultTimeout
	}
	warn := options.Warn
	if warn == nil {
		warn = func(string) {}
	}
	dial := options.Dial
	if dial == nil {
		dial = defaultEdgeDial
	}
	return &EdgeTTS{voices: options.Voices, now: now, timeout: timeout, warn: warn, dial: dial}
}

// Name is the Provider name.
func (e *EdgeTTS) Name() string { return "edge" }

func (e *EdgeTTS) timeoutError() error {
	return fmt.Errorf("Edge TTS timed out after %d ms. It is unofficial and unstable. %s",
		e.timeout.Milliseconds(), edgeFallback)
}

// Synthesize opens one read-aloud session and returns the joined mp3.
func (e *EdgeTTS) Synthesize(request protocol.TTSRequest) (protocol.Audio, error) {
	voices, err := voiceList("Edge", e.voices, request.Direction.Tgt, edgeFallback, e.warn)
	if err != nil {
		return protocol.Audio{}, err
	}
	voice := voices[0]
	lang := voiceLanguage(voice)
	if lang == "" {
		lang = request.Direction.Tgt
	}

	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()
	at := e.now()
	conn, response, err := e.dial(ctx, edgeSynthesisURL(at), &websocket.DialOptions{HTTPHeader: edgeHeaders()})
	if err != nil {
		if response != nil {
			return protocol.Audio{}, fmt.Errorf(
				"Edge TTS handshake was rejected (HTTP %d). "+
					"Microsoft changes this unofficial service without notice. %s",
				response.StatusCode, edgeFallback)
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return protocol.Audio{}, e.timeoutError()
		}
		return protocol.Audio{}, fmt.Errorf("Edge TTS connection failed: %v. It is unofficial and unstable. %s", err, edgeFallback)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	if err := conn.Write(ctx, websocket.MessageText, []byte(edgeConfigFrame(at))); err != nil {
		return protocol.Audio{}, fmt.Errorf("Edge TTS connection failed: %v. It is unofficial and unstable. %s", err, edgeFallback)
	}
	if err := conn.Write(ctx, websocket.MessageText, []byte(edgeSSMLFrame(request.Text, voice, lang, at))); err != nil {
		return protocol.Audio{}, fmt.Errorf("Edge TTS connection failed: %v. It is unofficial and unstable. %s", err, edgeFallback)
	}

	var chunks [][]byte
	for {
		messageType, data, err := conn.Read(ctx)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return protocol.Audio{}, e.timeoutError()
			}
			var closeError websocket.CloseError
			if errors.As(err, &closeError) {
				return protocol.Audio{}, fmt.Errorf(
					"Edge TTS closed the connection before finishing (code %d). "+
						"It is unofficial and unstable. %s", closeError.Code, edgeFallback)
			}
			return protocol.Audio{}, fmt.Errorf("Edge TTS connection failed: %v. It is unofficial and unstable. %s", err, edgeFallback)
		}
		if messageType == websocket.MessageText {
			if strings.Contains(string(data), "Path:turn.end") {
				audio := concatBytes(chunks)
				if len(audio) == 0 {
					return protocol.Audio{}, fmt.Errorf("Edge TTS returned no audio. %s", edgeFallback)
				}
				return protocol.Audio{Format: "mp3", Data: audio}, nil
			}
			continue
		}
		if len(data) < 2 {
			continue
		}
		headerLength := int(binary.BigEndian.Uint16(data))
		if len(data) < 2+headerLength {
			continue
		}
		if strings.Contains(string(data[2:2+headerLength]), "Path:audio") {
			chunks = append(chunks, data[2+headerLength:])
		}
	}
}

func edgeHeaders() http.Header {
	header := http.Header{}
	header.Set("Pragma", "no-cache")
	header.Set("Cache-Control", "no-cache")
	header.Set("Origin", "chrome-extension://jdiccldimpdaibmpdkjnbmckianbfold")
	header.Set("User-Agent", edgeUserAgent)
	header.Set("Accept-Encoding", "gzip, deflate, br")
	header.Set("Accept-Language", "en-US,en;q=0.9")
	return header
}

const edgeConfigPayload = `{"context":{"synthesis":{"audio":{"metadataoptions":` +
	`{"sentenceBoundaryEnabled":false,"wordBoundaryEnabled":false},` +
	`"outputFormat":"` + edgeOutputFormat + `"}}}}`

func edgeTimestamp(at time.Time) string {
	return at.UTC().Format("2006-01-02T15:04:05.000Z")
}

func edgeConfigFrame(at time.Time) string {
	return "X-Timestamp:" + edgeTimestamp(at) +
		"\r\nContent-Type:application/json; charset=utf-8\r\nPath:speech.config\r\n\r\n" + edgeConfigPayload
}

func edgeSSMLFrame(text, voice, lang string, at time.Time) string {
	ssml := "<speak version='1.0' xmlns='http://www.w3.org/2001/10/synthesis' xml:lang='" +
		escapeXML(lang) + "'><voice name='" + escapeXML(voice) + "'>" +
		escapeXML(text) + "</voice></speak>"
	return "X-RequestId:" + edgeConnectionID() +
		"\r\nContent-Type:application/ssml+xml\r\nX-Timestamp:" + edgeTimestamp(at) +
		"\r\nPath:ssml\r\n\r\n" + ssml
}

func edgeSynthesisURL(now time.Time) string {
	return edgeEndpoint + "?TrustedClientToken=" + edgeTrustedClientToken +
		"&Sec-MS-GEC=" + edgeSecMsGec(now) +
		"&Sec-MS-GEC-Version=" + edgeSecGecVersion +
		"&ConnectionId=" + edgeConnectionID()
}

func edgeConnectionID() string {
	value := make([]byte, 16)
	_, _ = rand.Read(value)
	return hex.EncodeToString(value)
}

// Sec-MS-GEC v2 is CS64(copyright + ticks + token) + "-" + SHA-256(same input).
var (
	edgeSwap = [2][3]uint32{
		{0xef0569fb, 0x76f50f34, 0xc3efea97},
		{0xc31713db, 0x1881ce3c, 0x35bd1ec9},
	}
	edgeMix = [2][4]uint32{
		{0xcf98b111, 0x87085b9f, 0x12ceb96d, 0x257e1d83},
		{0xa27416f5, 0xd38396ff, 0x7c932b89, 0xbfa49f69},
	}
)

func edgeSwap16(value uint32) uint32 { return value<<16 | value>>16 }

func edgeSwapTerm(word, key uint32, multipliers [3]uint32) uint32 {
	sum := word*key + edgeSwap16(word)*multipliers[0]
	product := edgeSwap16(sum) * multipliers[1]
	return edgeSwap16(product)*multipliers[2] + product
}

func edgeMixWord(value uint32, multipliers [4]uint32) uint32 {
	for _, multiplier := range multipliers {
		value = edgeSwap16(value) * multiplier
	}
	return value
}

func edgeCS64(message []byte, digest [32]byte) []byte {
	keys := [2]uint32{
		binary.LittleEndian.Uint32(digest[0:]) | 1,
		binary.LittleEndian.Uint32(digest[4:]) | 1,
	}
	padded := make([]byte, (len(message)+7)&^7)
	copy(padded, message)

	var swapSum, lastSwap, mix, mixSum uint32
	for offset := 0; offset < len(padded); offset += 8 {
		words := [2]uint32{
			binary.LittleEndian.Uint32(padded[offset:]),
			binary.LittleEndian.Uint32(padded[offset+4:]),
		}
		swapSum += edgeSwapTerm(words[0], keys[0], edgeSwap[0]) +
			edgeSwapTerm(words[1], keys[1], edgeSwap[1])
		lastSwap = edgeSwapTerm(words[1], keys[1], edgeSwap[1])
		for i := 0; i < 2; i++ {
			mix = edgeMixWord((mix+words[i])*keys[i], edgeMix[i])
			mixSum += mix
		}
	}

	out := make([]byte, 8)
	binary.LittleEndian.PutUint32(out[0:], lastSwap^mix)
	binary.LittleEndian.PutUint32(out[4:], swapSum^mixSum)
	return out
}

func edgeSecMsGec(now time.Time) string {
	seconds := now.Unix()
	rounded := seconds - seconds%300
	ticks := (rounded + 11644473600) * 10000000
	message := edgeCopyright + strconv.FormatInt(ticks, 10) + edgeTrustedClientToken
	digest := sha256.Sum256([]byte(message))
	return strings.ToUpper(hex.EncodeToString(edgeCS64([]byte(message), digest))) +
		"-" + strings.ToUpper(hex.EncodeToString(digest[:]))
}
