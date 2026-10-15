package main

import (
	"encoding/json"
	"testing"
)

func TestCompactAXTreeDropsUnnamedGeneric(t *testing.T) {
	raw := json.RawMessage(`{
		"nodes": [
			{"backendDOMNodeId": 1, "role": {"value": "generic"}},
			{"backendDOMNodeId": 2, "role": {"value": "button"}, "name": {"value": "Go"}, "bounds": {"x": 1, "y": 2, "width": 10, "height": 20}}
		]
	}`)
	nodes, err := compactAXTree(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Fatalf("nodes = %d, want 1", len(nodes))
	}
	if nodes[0].Ref != "e1" || nodes[0].Role != "button" || nodes[0].Name != "Go" {
		t.Fatalf("unexpected node: %+v", nodes[0])
	}
	snapshotMu.Lock()
	id := snapshotRefs["e1"]
	snapshotMu.Unlock()
	if id != 2 {
		t.Errorf("backend id = %d, want 2", id)
	}
}

func TestKeepAXNode(t *testing.T) {
	if keepAXNode("generic", "", "") {
		t.Error("generic with no name should be dropped")
	}
	if !keepAXNode("link", "Home", "") {
		t.Error("named link should be kept")
	}
}
