package mcp

import (
	"encoding/json"
	"testing"
)

func TestBuildInspectSummary(t *testing.T) {
	t.Parallel()
	snap := []byte(`{"nodes":[{"ref":"e1","role":"RootWebArea","name":"example.com"},{"ref":"e2","role":"Iframe","name":"captcha"}]}`)
	net := []byte(`{"entries":[{"url":"https://www.example.com/","type":"Document","status":403,"method":"GET"},{"url":"https://cdn.example/x.js","type":"Script","status":200}]}`)
	out := buildInspectSummary(snap, net)
	if out["page_title"] != "example.com" {
		t.Fatalf("page_title %v", out["page_title"])
	}
	if out["nodes"] != 2 {
		t.Fatalf("nodes %v", out["nodes"])
	}
	netPart, ok := out["network"].(map[string]any)
	if !ok || netPart["entry_count"] != 2 {
		t.Fatalf("network summary: %v", out["network"])
	}
	docs, ok := netPart["documents"].([]map[string]any)
	if !ok || len(docs) != 1 || docs[0]["status"] != 403 {
		b, _ := json.Marshal(netPart["documents"])
		t.Fatalf("documents: %s", b)
	}
}
