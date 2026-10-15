package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

const snapshotMaxNodes = 200

// snapshotNode is a compact accessibility node for MCP / automation.
type snapshotNode struct {
	Ref   string `json:"ref"`
	Role  string `json:"role"`
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
	X     int    `json:"x"`
	Y     int    `json:"y"`
	W     int    `json:"w"`
	H     int    `json:"h"`
}

var snapshotMu sync.Mutex
var snapshotRefs = map[string]int64{} // ref -> backendDOMNodeId

var interactiveRoles = map[string]bool{
	"button": true, "link": true, "textbox": true, "searchbox": true,
	"checkbox": true, "radio": true, "combobox": true, "listbox": true,
	"menuitem": true, "option": true, "tab": true, "switch": true,
	"slider": true, "spinbutton": true,
}

func axValue(v *struct {
	Value any `json:"value"`
}) string {
	if v == nil {
		return ""
	}
	switch val := v.Value.(type) {
	case string:
		return val
	case float64:
		return fmt.Sprintf("%v", val)
	default:
		return ""
	}
}

func keepAXNode(role, name, value string) bool {
	if interactiveRoles[role] {
		return true
	}
	if name != "" || value != "" {
		return true
	}
	return false
}

func compactAXTree(raw json.RawMessage) ([]snapshotNode, error) {
	var tree struct {
		Nodes []struct {
			BackendDOMNodeId int64 `json:"backendDOMNodeId"`
			Role             *struct {
				Value string `json:"value"`
			} `json:"role"`
			Name *struct {
				Value any `json:"value"`
			} `json:"name"`
			Value *struct {
				Value any `json:"value"`
			} `json:"value"`
			Bounds *struct {
				X      float64 `json:"x"`
				Y      float64 `json:"y"`
				Width  float64 `json:"width"`
				Height float64 `json:"height"`
			} `json:"bounds"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(raw, &tree); err != nil {
		return nil, err
	}

	refs := make(map[string]int64)
	out := make([]snapshotNode, 0, snapshotMaxNodes)
	n := 0
	for _, node := range tree.Nodes {
		if node.BackendDOMNodeId == 0 {
			continue
		}
		role := ""
		if node.Role != nil {
			role = node.Role.Value
		}
		name := axValue(node.Name)
		value := axValue(node.Value)
		if !keepAXNode(role, name, value) {
			continue
		}
		n++
		ref := fmt.Sprintf("e%d", n)
		refs[ref] = node.BackendDOMNodeId
		sn := snapshotNode{Ref: ref, Role: role, Name: name, Value: value}
		if node.Bounds != nil {
			sn.X = int(node.Bounds.X)
			sn.Y = int(node.Bounds.Y)
			sn.W = int(node.Bounds.Width)
			sn.H = int(node.Bounds.Height)
		}
		out = append(out, sn)
		if len(out) >= snapshotMaxNodes {
			break
		}
	}
	snapshotMu.Lock()
	snapshotRefs = refs
	snapshotMu.Unlock()
	return out, nil
}

func snapshotHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if cdpClient == nil || !cdpClient.Available() {
		http.Error(w, "cdp not available", http.StatusServiceUnavailable)
		return
	}
	nodes, err := refreshSnapshotRefs()
	if err != nil {
		log.Printf("snapshot: %v", err)
		http.Error(w, "snapshot failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"nodes": nodes})
}

// refreshSnapshotRefs reads the current accessibility tree and repopulates the
// ref map, returning the compact nodes. Both /snapshot and /links use it: links
// refresh first so any ref they return is actionable for click on this page
// state, rather than a leftover from an earlier navigation.
func refreshSnapshotRefs() ([]snapshotNode, error) {
	if cdpClient == nil || !cdpClient.Available() {
		return nil, fmt.Errorf("cdp not available")
	}
	_ = cdpClient.send("Accessibility.enable", map[string]interface{}{})
	raw, err := cdpClient.callWithResult("Accessibility.getFullAXTree", map[string]interface{}{}, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("getFullAXTree: %w", err)
	}
	return compactAXTree(raw)
}

func resolveRefCenter(ref string) (int, int, bool) {
	snapshotMu.Lock()
	backendID, ok := snapshotRefs[ref]
	snapshotMu.Unlock()
	if !ok || backendID == 0 {
		return 0, 0, false
	}
	if cdpClient == nil || !cdpClient.Available() {
		return 0, 0, false
	}
	raw, err := cdpClient.callWithResult("DOM.getBoxModel", map[string]interface{}{
		"backendNodeId": backendID,
	}, 3*time.Second)
	if err != nil {
		log.Printf("click: getBoxModel: %v", err)
		return 0, 0, false
	}
	var model struct {
		Model struct {
			Content []float64 `json:"content"`
		} `json:"model"`
	}
	if json.Unmarshal(raw, &model) != nil || len(model.Model.Content) < 8 {
		return 0, 0, false
	}
	c := model.Model.Content
	x := int((c[0] + c[2] + c[4] + c[6]) / 4)
	y := int((c[1] + c[3] + c[5] + c[7]) / 4)
	return x, y, true
}

func clickAt(x, y int) {
	ev := inputEvent{Type: "mousedown", Button: "left", X: x, Y: y}
	handleInputEvent(mustJSON(ev))
	ev.Type = "mouseup"
	handleInputEvent(mustJSON(ev))
}

func mcpClickHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Ref string `json:"ref"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Ref) == "" {
		http.Error(w, "ref required", http.StatusBadRequest)
		return
	}
	x, y, ok := resolveRefCenter(strings.TrimSpace(body.Ref))
	if !ok {
		http.Error(w, "unknown ref", http.StatusBadRequest)
		return
	}
	clickAt(x, y)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func mcpTypeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Ref  string `json:"ref"`
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Text == "" {
		http.Error(w, "text required", http.StatusBadRequest)
		return
	}
	if ref := strings.TrimSpace(body.Ref); ref != "" {
		x, y, ok := resolveRefCenter(ref)
		if !ok {
			http.Error(w, "unknown ref", http.StatusBadRequest)
			return
		}
		clickAt(x, y)
	}
	handleInputEvent(mustJSON(inputEvent{Type: "type", Text: body.Text}))
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
