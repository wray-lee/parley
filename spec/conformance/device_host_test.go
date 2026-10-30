package conformance

import (
	"encoding/json"
	"testing"

	"github.com/wray-lee/parley/mobile"
)

// TestConformanceAgainstDeviceHost runs every case against the exact Start
// surface gomobile binds for the apps, in the desktop harness. Demo Mode comes
// from the empty config: no keys means mock providers.
func TestConformanceAgainstDeviceHost(t *testing.T) {
	raw, err := mobile.Start(`{}`)
	if err != nil {
		t.Fatalf("mobile.Start: %v", err)
	}
	t.Cleanup(mobile.Stop)

	var endpoint struct {
		URL   string `json:"url"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(raw), &endpoint); err != nil {
		t.Fatalf("mobile.Start returned %q: %v", raw, err)
	}

	results := runSuite(endpoint.URL+"?token="+endpoint.Token, options{originReject: true})
	logResults(t, "device host", results)
	assertNoFailures(t, "device host", results)
}
