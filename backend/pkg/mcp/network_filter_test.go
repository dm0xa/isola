package mcp

import (
	"encoding/json"
	"testing"
)

func TestFilterNetworkBody(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"entries":[
		{"url":"https://www.example.com/","type":"Document","status":403},
		{"url":"https://ads.evil.example/i.js","type":"Script","status":200}
	]}`)
	out, err := filterNetworkBody(raw, "Document", "example.com", 10)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Entries []networkEntry `json:"entries"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Entries) != 1 || parsed.Entries[0].Status != 403 {
		t.Fatalf("entries: %+v", parsed.Entries)
	}
}
