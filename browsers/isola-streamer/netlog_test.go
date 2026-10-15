package main

import (
	"encoding/json"
	"testing"
)

func TestNetMonitorStoresProtocolAndConnectionID(t *testing.T) {
	netMu.Lock()
	netRequests = map[string]*netRequest{}
	netOrder = []string{}
	netMu.Unlock()

	params := map[string]any{
		"requestId":    "42",
		"type":         "XHR",
		"connectionId": 99,
		"request": map[string]string{
			"url":    "https://example.com/" + string(make([]byte, 400)),
			"method": "GET",
		},
		"response": map[string]any{
			"status":          200,
			"mimeType":        "application/json",
			"protocol":        "h2",
			"remoteIPAddress": "93.184.216.34",
		},
	}
	reqParams := map[string]any{
		"requestId": "42",
		"type":      "XHR",
		"request": map[string]string{
			"url":    "https://example.com/" + string(make([]byte, 400)),
			"method": "GET",
		},
	}
	reqRaw, _ := json.Marshal(reqParams)
	netMonitorTap("target-aaaa", "", "Network.requestWillBeSent", reqRaw, 1)
	raw, _ := json.Marshal(params)
	netMonitorTap("target-aaaa", "", "Network.responseReceived", raw, 1)

	entries := snapshotNetworkLog()
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.Protocol != "h2" {
		t.Errorf("protocol = %q, want h2", e.Protocol)
	}
	if e.ConnectionID != 99 {
		t.Errorf("connection_id = %d, want 99", e.ConnectionID)
	}
	if e.Mime != "application/json" {
		t.Errorf("mime = %q", e.Mime)
	}
	if len(e.URL) < 300 {
		t.Errorf("expected full URL in dump, got len %d", len(e.URL))
	}
}
