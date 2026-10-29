// Package settings validates Provider settings entered in a client. The case
// table lives in spec/fixtures/settings-validation.json and is shared with the
// Flutter client, so a rule cannot drift between the two hosts.
package settings

import (
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strings"
)

// Error is a stable validation code (the fixture's `error` strings).
type Error string

func (e Error) Error() string { return string(e) }

const (
	ErrNeedsBaseURL Error = "needsBaseUrl"
	ErrInvalidURL   Error = "invalidUrl"
	ErrNotHTTP      Error = "notHttp"
	ErrPlainHTTP    Error = "plainHttp"
	ErrCredentials  Error = "credentials"
	ErrQuery        Error = "query"
	ErrNeedsModel   Error = "needsModel"
	ErrInvalidJSON  Error = "invalidJson"
	ErrNotObject    Error = "notObject"
)

// NormalizeBaseURL trims a Base URL and applies the v0.0.2 rules: https
// anywhere, plain http only on localhost or a private LAN IPv4 address, no
// credentials, query or fragment. An empty value is allowed unless the custom
// Preset is selected.
func NormalizeBaseURL(value, preset string) (string, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(value), "/")
	if baseURL == "" {
		if preset == "custom" {
			return "", ErrNeedsBaseURL
		}
		return "", nil
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", ErrInvalidURL
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", ErrNotHTTP
	}
	if parsed.User != nil {
		return "", ErrCredentials
	}
	if strings.ContainsAny(baseURL, "?#") {
		return "", ErrQuery
	}
	if parsed.Scheme == "http" && !isPrivateHTTPHost(parsed.Hostname()) {
		return "", ErrPlainHTTP
	}
	return baseURL, nil
}

// NormalizeModel trims a model name. The custom Preset needs one.
func NormalizeModel(value, preset string) (string, error) {
	model := strings.TrimSpace(value)
	if model == "" && preset == "custom" {
		return "", ErrNeedsModel
	}
	return model, nil
}

// NormalizeExtraBody trims optional extra request fields and requires a JSON
// object when non-empty.
func NormalizeExtraBody(value string) (string, error) {
	text := strings.TrimSpace(value)
	if text == "" {
		return "", nil
	}
	var parsed any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return "", ErrInvalidJSON
	}
	if _, ok := parsed.(map[string]any); !ok {
		return "", ErrNotObject
	}
	return text, nil
}

// NeedsLocalNetworkPermission reports whether saving or connecting would reach
// a Provider over the LAN, which Android 17 gates behind a runtime permission.
func NeedsLocalNetworkPermission(values map[string]string) bool {
	if values["mode"] == "server" {
		return isLocalNetworkURL(values["serverUrl"])
	}
	if isLocalNetworkURL(values["translatorBaseUrl"]) {
		return true
	}
	return values["ttsProvider"] == "kokoro" && isLocalNetworkURL(values["kokoroUrl"])
}

func isPrivateHTTPHost(hostname string) bool {
	// Hostnames are case-insensitive; Dart's Uri and the JS URL parser both
	// lowercase them, so "http://LOCALHOST" is the same as "localhost".
	hostname = strings.ToLower(hostname)
	if hostname == "localhost" {
		return true
	}
	ip := net.ParseIP(hostname)
	if ip == nil || ip.To4() == nil {
		return false
	}
	a, b := ip.To4()[0], ip.To4()[1]
	return a == 10 || a == 127 || (a == 169 && b == 254) || (a == 192 && b == 168) || (a == 172 && b >= 16 && b <= 31)
}

func isLocalNetworkURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || host == "::1" || strings.HasPrefix(host, "127.") {
		return false
	}
	if strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".lan") || strings.HasSuffix(host, ".home.arpa") {
		return true
	}
	if isPrivateHTTPHost(host) {
		return true
	}
	ip := net.ParseIP(host)
	if ip != nil {
		if ip.IsLoopback() {
			return false
		}
		if ip.IsPrivate() || ip.IsLinkLocalUnicast() {
			return true
		}
		v4 := ip.To4()
		if v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
			return true // CGNAT 100.64.0.0/10
		}
	}
	return false
}

// IsValidationError reports whether err is one of the stable fixture codes.
func IsValidationError(err error) bool {
	var target Error
	return errors.As(err, &target)
}
