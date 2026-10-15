// In-process e2e test for isola-streamer. Starts the streamer server
// in-process, connects a mock frame client and WS viewer, and verifies
// the full pipeline: frame UDS → broadcaster → ffmpeg → fMP4 → WS.
//
// Run with: go test -v -run TestE2E ./...
package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

const (
	e2eTestSockPath = "/tmp/isola-e2e-inproc.sock"
	e2eTestPort     = "15910"
	e2eTestWidth    = 640
	e2eTestHeight   = 480
)

func TestE2EInProcess(t *testing.T) {
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	os.Remove(e2eTestSockPath)

	// Start the streamer in-process (same package, so we can call main
	// functions directly).
	// Set config vars and env vars since we bypass main().
	currentMu.Lock()
	currentWidth = e2eTestWidth
	currentHeight = e2eTestHeight
	currentFPS = 15
	currentBitrateKbps = 500
	currentMu.Unlock()
	os.Setenv("ISOLA_FRAME_SOCK", e2eTestSockPath)
	os.Setenv("ISOLA_WIDTH", fmt.Sprint(e2eTestWidth))
	os.Setenv("ISOLA_HEIGHT", fmt.Sprint(e2eTestHeight))
	os.Setenv("ISOLA_FPS", "15")
	os.Setenv("ISOLA_BITRATE_KBPS", "500")

	// Start the UDS listener
	incoming, err := listenFrames(e2eTestSockPath)
	if err != nil {
		t.Fatalf("listenFrames: %v", err)
	}
	globalBroadcaster = newFrameBroadcaster(incoming)
	t.Logf("frame broadcaster ready (socket=%s) ✓", e2eTestSockPath)

	// Start the HTTP/WS mux on the test port
	listener, err := setupMux(e2eTestPort, func(w http.ResponseWriter, r *http.Request) {
		if !websocket.IsWebSocketUpgrade(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
			return
		}
		handleViewerWS(w, r)
	})
	if err != nil {
		t.Fatalf("setupMux: %v", err)
	}
	defer listener.Close()
	t.Log("mux started ✓")

	// Connect WS viewer FIRST (before mock client) so the broadcaster
	// already has a subscriber when frames start arriving. This ensures
	// the latest frame is buffered and delivered immediately.
	wsURL := "ws://127.0.0.1:" + e2eTestPort + "/"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WS dial: %v", err)
	}
	defer conn.Close()
	t.Log("WS connected ✓")

	// Read stream_init (must arrive within 5s)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	msgType, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read stream_init: %v", err)
	}
	if msgType != websocket.TextMessage {
		t.Fatalf("expected text for stream_init, got %d", msgType)
	}
	var init struct {
		Type   string `json:"type"`
		Codec  string `json:"codec"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
	}
	if err := json.Unmarshal(data, &init); err != nil {
		t.Fatalf("parse stream_init: %v", err)
	}
	if init.Type != "stream_init" {
		t.Fatalf("expected stream_init, got %s", init.Type)
	}
	if init.Codec == "" {
		t.Fatal("missing codec")
	}
	t.Logf("stream_init: codec=%s %dx%d ✓", init.Codec, init.Width, init.Height)

	// Start mock frame client AFTER WS is connected, so the broadcaster
	// already has a subscriber and frames flow immediately to ffmpeg.
	go runFrameClientInProc(t, e2eTestSockPath)
	time.Sleep(300 * time.Millisecond)

	// Give ffmpeg time to produce the first fMP4 segment (1 GOP = 1s at 15fps)
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	segCount := 0
	for segCount < 5 {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read segment %d: %v", segCount, err)
		}
		if mt == websocket.TextMessage {
			continue
		}
		if len(data) < 10 {
			t.Fatalf("segment too short: %d bytes", len(data))
		}
		if data[0] != 0x01 {
			t.Fatalf("bad marker: 0x%02x", data[0])
		}
		payloadLen := binary.LittleEndian.Uint32(data[6:10])
		if int(payloadLen) != len(data)-10 {
			t.Fatalf("payloadLen=%d != actual=%d", payloadLen, len(data)-10)
		}
		segCount++
		if segCount == 1 {
			seq := binary.LittleEndian.Uint32(data[2:6])
			isKey := (data[1] & 0x01) != 0
			t.Logf("[seg #1] seq=%d key=%v payloadLen=%d ✓", seq, isKey, payloadLen)
		}
	}
	t.Logf("received %d video segments ✓", segCount)

	// Send input event
	err = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"mousemove","x":100,"y":200}`))
	if err != nil {
		t.Errorf("input send: %v", err)
	} else {
		t.Log("input event sent ✓")
	}

	os.Remove(e2eTestSockPath)
}

func runFrameClientInProc(t *testing.T, sockPath string) {
	for i := 0; i < 80; i++ {
		conn, err := net.Dial("unix", sockPath)
		if err == nil {
			defer conn.Close()
			t.Log("frame client connected to UDS ✓")
			sendTestFrames(conn, e2eTestWidth, e2eTestHeight)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Error("frame client: failed to connect to UDS after retries")
}

func sendTestFrames(conn net.Conn, width, height int) {
	ticker := time.NewTicker(time.Second / 30)
	defer ticker.Stop()
	frameNum := 0
	stride := uint32(width * 4)

	for range ticker.C {
		pixels := make([]byte, height*int(stride))
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				off := y*int(stride) + x*4
				pixels[off] = uint8(x * 255 / width)
				pixels[off+1] = uint8(y * 255 / height)
				pixels[off+2] = uint8(frameNum % 256)
				pixels[off+3] = 255
			}
		}

		hdr := make([]byte, 32)
		binary.LittleEndian.PutUint32(hdr[0:4], 0x4C4F5349)
		binary.LittleEndian.PutUint32(hdr[4:8], uint32(width))
		binary.LittleEndian.PutUint32(hdr[8:12], uint32(height))
		binary.LittleEndian.PutUint32(hdr[12:16], stride)
		binary.LittleEndian.PutUint32(hdr[20:24], 1)
		binary.LittleEndian.PutUint64(hdr[24:32], uint64(time.Now().UnixNano()))

		dirty := make([]byte, 16)
		binary.LittleEndian.PutUint32(dirty[0:4], 0)
		binary.LittleEndian.PutUint32(dirty[4:8], 0)
		binary.LittleEndian.PutUint32(dirty[8:12], uint32(width))
		binary.LittleEndian.PutUint32(dirty[12:16], uint32(height))

		conn.Write(hdr)
		conn.Write(dirty)
		conn.Write(pixels)
		frameNum++
		if frameNum > 300 {
			return
		}
	}
}
