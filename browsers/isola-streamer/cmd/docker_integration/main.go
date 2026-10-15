// Docker integration test: create isola session via the isola API, verify fMP4 over proxied WS.
// Run: go run ./cmd/docker_integration/ [base_url]
package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	base := "http://localhost:8080"
	if len(os.Args) > 1 {
		base = os.Args[1]
	}

	log.Println("=== isola Docker integration test ===")
	log.Printf("isola base: %s", base)

	// 1. Create session
	body, _ := json.Marshal(map[string]any{
		"url":        "https://example.com",
		"resolution": "1280x720",
		"transport":  "isola",
	})
	resp, err := http.Post(base+"/api/sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		log.Fatalf("create session: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		log.Fatalf("create session: HTTP %d", resp.StatusCode)
	}
	var created struct {
		ID           string `json:"id"`
		SessionToken string `json:"session_token"`
		Transport    string `json:"transport"`
		Status       string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		log.Fatalf("decode create: %v", err)
	}
	log.Printf("session created: id=%s transport=%s status=%s", created.ID, created.Transport, created.Status)

	// 2. Poll until running
	deadline := time.Now().Add(90 * time.Second)
	var session struct {
		Status    string `json:"status"`
		Transport string `json:"transport"`
	}
	for time.Now().Before(deadline) {
		r, err := http.Get(base + "/api/sessions/" + created.ID)
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		json.NewDecoder(r.Body).Decode(&session)
		r.Body.Close()
		log.Printf("  poll status=%s transport=%s", session.Status, session.Transport)
		if session.Status == "running" {
			break
		}
		if session.Status == "error" || session.Status == "terminated" {
			log.Fatalf("session failed: %s", session.Status)
		}
		time.Sleep(2 * time.Second)
	}
	if session.Status != "running" {
		log.Fatalf("timeout waiting for running (last=%s)", session.Status)
	}
	log.Println("session running ✓")

	// 3. Connect WS through the isola proxy
	wsURL := fmt.Sprintf("ws://localhost:8080/ws/%s?channel=isola&host=1&session_token=%s",
		created.ID, created.SessionToken)
	if base != "http://localhost:8080" {
		// crude ws rewrite for non-default host
		wsURL = fmt.Sprintf("ws://%s/ws/%s?channel=isola&host=1&session_token=%s",
			stripHTTP(base), created.ID, created.SessionToken)
	}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		log.Fatalf("ws dial: %v", err)
	}
	defer conn.Close()
	log.Println("WS connected via proxy ✓")
	conn.SetReadDeadline(time.Now().Add(15 * time.Second))

	var streamInit bool
	var segments int
	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() && segments > 0 {
				break
			}
			if segments == 0 {
				log.Fatalf("ws read: %v", err)
			}
			break
		}
		if mt == websocket.TextMessage {
			var msg map[string]any
			if json.Unmarshal(data, &msg) == nil && msg["type"] == "stream_init" {
				streamInit = true
				log.Printf("stream_init: %v ✓", msg)
			}
			continue
		}
		if mt == websocket.BinaryMessage && len(data) > 10 {
			segments++
			seq := binary.LittleEndian.Uint32(data[2:6])
			payloadLen := binary.LittleEndian.Uint32(data[6:10])
			if segments == 1 {
				log.Printf("segment #1: seq=%d payload=%d bytes ✓", seq, payloadLen)
			}
			if segments >= 5 {
				break
			}
		}
	}

	if !streamInit {
		log.Fatal("FAIL: no stream_init")
	}
	if segments == 0 {
		log.Fatal("FAIL: no video segments")
	}
	if segments < 2 {
		log.Fatal("FAIL: only init segment received — ffmpeg needs sustained frames")
	}
	log.Printf("received %d video segments ✓", segments)

	// 4. Check container logs hint
	log.Println()
	log.Println("╔══════════════════════════════════════════╗")
	log.Println("║  DOCKER INTEGRATION PASSED              ║")
	log.Printf("║  Viewer: %s/run/%s", base, created.ID)
	log.Println("╚══════════════════════════════════════════╝")
}

func stripHTTP(u string) string {
	if len(u) > 7 && u[:7] == "http://" {
		return u[7:]
	}
	if len(u) > 8 && u[:8] == "https://" {
		return u[8:]
	}
	return u
}
