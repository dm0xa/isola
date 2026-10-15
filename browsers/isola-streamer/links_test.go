package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestBuildLinksExpression(t *testing.T) {
	expr := buildLinksExpression(50)
	if !strings.Contains(expr, `querySelectorAll("a[href]")`) {
		t.Errorf("selector not embedded: %s", expr)
	}
	if !strings.Contains(expr, "out.length < 50") {
		t.Errorf("cap not applied: %s", expr)
	}
	if !strings.Contains(expr, "i: i") {
		t.Errorf("document-order index not reported: %s", expr)
	}
}

func TestSelectLinks(t *testing.T) {
	raw := []rawLink{
		{Idx: 0, Href: "https://a.example/x", Text: "  X  "},
		{Idx: 1, Href: "https://a.example/x", Text: "duplicate"},
		{Idx: 2, Href: "https://b.example/y", Text: "Y"},
		{Idx: 3, Href: "", Text: "no href"},
	}

	got := selectLinks(raw, "", 10)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (dedupe + drop empty): %+v", len(got), got)
	}
	if got[0].Href != "https://a.example/x" || got[0].Text != "X" || got[0].idx != 0 {
		t.Errorf("first = %+v", got[0])
	}
	if got[1].Href != "https://b.example/y" || got[1].idx != 2 {
		t.Errorf("second = %+v", got[1])
	}

	if filtered := selectLinks(raw, "b.example", 10); len(filtered) != 1 || filtered[0].Href != "https://b.example/y" {
		t.Errorf("host filter = %+v", filtered)
	}
	if capped := selectLinks(raw, "", 1); len(capped) != 1 || capped[0].Href != "https://a.example/x" {
		t.Errorf("cap = %+v", capped)
	}
}

func TestLinksHandlerPairsRefs(t *testing.T) {
	const evalPayload = `{"url":"https://example.com/","title":"Example","links":[{"i":0,"href":"https://example.com/a","text":"A"}]}`
	const axPayload = `{"nodes":[{"backendDOMNodeId":77,"role":{"value":"link"},"name":{"value":"A"}}]}`

	srv := newFakeCDP(t, evalPayload, axPayload, []int64{11}, 77)
	defer srv.Close()

	attachFakeCDPClient(t, srv.URL)

	rec := httptest.NewRecorder()
	linksHandler(rec, httptest.NewRequest(http.MethodGet, "/links?limit=5", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"href":"https://example.com/a"`) {
		t.Fatalf("missing href: %s", body)
	}
	if !strings.Contains(body, `"ref":"e1"`) {
		t.Fatalf("ref not paired with the snapshot node: %s", body)
	}

	rec = httptest.NewRecorder()
	linksHandler(rec, httptest.NewRequest(http.MethodPost, "/links", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want 405", rec.Code)
	}

	rec = httptest.NewRecorder()
	linksHandler(rec, httptest.NewRequest(http.MethodGet, "/links?limit=0", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("limit=0 status = %d, want 400", rec.Code)
	}
}

// Without any accessibility nodes the endpoint must still return links, just
// without refs — ref pairing is best-effort.
func TestLinksHandlerWithoutRefsStillReturnsLinks(t *testing.T) {
	const evalPayload = `{"url":"https://example.com/","title":"Example","links":[{"i":0,"href":"https://example.com/a","text":"A"}]}`

	srv := newFakeCDP(t, evalPayload, `{"nodes":[]}`, []int64{11}, 77)
	defer srv.Close()

	attachFakeCDPClient(t, srv.URL)

	rec := httptest.NewRecorder()
	linksHandler(rec, httptest.NewRequest(http.MethodGet, "/links", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"href":"https://example.com/a"`) {
		t.Fatalf("missing href: %s", body)
	}
	if strings.Contains(body, `"ref"`) {
		t.Fatalf("unexpected ref: %s", body)
	}
}

// attachFakeCDPClient points the package-level client at the fake server and
// waits until it is attached to the fake page.
func attachFakeCDPClient(t *testing.T, serverURL string) {
	t.Helper()
	c := newCDPClient()
	c.baseURL = serverURL
	prev := cdpClient
	cdpClient = c
	t.Cleanup(func() { cdpClient = prev })
	go c.Start()
	waitForAttached(t, c, "https://example.com/")
}

// newFakeCDP serves /json/list and answers the CDP commands /links and
// /snapshot use: the accessibility tree, the page-evaluated anchors, and the
// DOM node->backendNodeId lookups used for ref pairing.
func newFakeCDP(t *testing.T, evalValue, axValue string, anchorNodeIDs []int64, backendID int64) *httptest.Server {
	t.Helper()
	up := websocket.Upgrader{}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/json/list" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"type":                 "page",
				"url":                  "https://example.com/",
				"webSocketDebuggerUrl": "ws://" + r.Host + "/p/main",
			}})
			return
		}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var req struct {
				ID     int    `json:"id"`
				Method string `json:"method"`
			}
			if json.Unmarshal(raw, &req) != nil || req.ID == 0 {
				continue
			}
			var result any
			switch req.Method {
			case "Runtime.evaluate":
				result = map[string]any{"result": map[string]any{"type": "string", "value": evalValue}}
			case "Accessibility.getFullAXTree":
				result = json.RawMessage(axValue)
			case "DOM.getDocument":
				result = map[string]any{"root": map[string]any{"nodeId": 1}}
			case "DOM.querySelectorAll":
				result = map[string]any{"nodeIds": anchorNodeIDs}
			case "DOM.describeNode":
				result = map[string]any{"node": map[string]any{"backendNodeId": backendID}}
			default:
				result = map[string]any{}
			}
			_ = conn.WriteJSON(map[string]any{"id": req.ID, "result": result})
		}
	}))
}
