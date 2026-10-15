package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestUXAndLatencyBenchmark(t *testing.T) {
	const port = "15930"
	os.Setenv("ISOLA_STATIC_KEEPALIVE_MS", "2000")
	listener := startTestStreamer(t, port, 2000)
	defer listener.Close()

	uds, err := net.Dial("unix", e2eTestSockPath)
	if err != nil {
		t.Fatalf("UDS dial: %v", err)
	}
	defer uds.Close()

	w, h := e2eTestWidth, e2eTestHeight
	plane := make([]byte, w*h*4)
	for i := 0; i < len(plane); i += 4 {
		plane[i] = 128
		plane[i+1] = 128
		plane[i+2] = 128
		plane[i+3] = 255
	}

	if err := writeFrame(uds, w, h, plane); err != nil {
		t.Fatalf("send initial frame: %v", err)
	}

	// 1. Initial connection & stream_init
	t0 := time.Now()
	conn, initMsg := dialWebcodecsViewer(t, port)
	defer conn.Close()
	tInit := time.Now()

	// Wait for first packet
	var tFirstPacket time.Time
	for time.Now().Before(t0.Add(5 * time.Second)) {
		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		mt, _, err := conn.ReadMessage()
		if err == nil && mt == websocket.BinaryMessage {
			tFirstPacket = time.Now()
			break
		}
	}
	handshakeLatency := tInit.Sub(t0)
	ttfp := tFirstPacket.Sub(t0)

	// 2. Ping-Pong RTT
	pingT0 := time.Now()
	testTs := fmt.Sprintf("%d", pingT0.UnixNano())
	if err := conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(`{"type":"ping","ts":%s}`, testTs))); err != nil {
		t.Fatalf("send ping: %v", err)
	}
	var pongReceived time.Time
	for time.Now().Before(pingT0.Add(2 * time.Second)) {
		_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		mt, data, err := conn.ReadMessage()
		if err == nil && mt == websocket.TextMessage {
			var msg map[string]any
			if json.Unmarshal(data, &msg) == nil && msg["type"] == "pong" {
				pongReceived = time.Now()
				break
			}
		}
	}
	rtt := pongReceived.Sub(pingT0)

	// 3. Interaction Delivery Latency: simulate a change landing in UDS
	for i := 0; i < len(plane); i += 4 {
		plane[i] = 0
		plane[i+1] = 255
		plane[i+2] = 0
		plane[i+3] = 255
	}
	changeT0 := time.Now()
	if err := writeFrame(uds, w, h, plane); err != nil {
		t.Fatalf("send change frame: %v", err)
	}
	var changeReceived time.Time
	for time.Now().Before(changeT0.Add(2 * time.Second)) {
		_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		mt, _, err := conn.ReadMessage()
		if err == nil && mt == websocket.BinaryMessage {
			changeReceived = time.Now()
			break
		}
	}
	changeLatency := changeReceived.Sub(changeT0)

	// 4. Keyframe On-Demand Recovery
	reqT0 := time.Now()
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"request_keyframe"}`)); err != nil {
		t.Fatalf("send request_keyframe: %v", err)
	}
	var keyReceived time.Time
	var sawReset, sawInit bool
	for time.Now().Before(reqT0.Add(3 * time.Second)) {
		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		mt, data, err := conn.ReadMessage()
		if err != nil {
			continue
		}
		if mt == websocket.TextMessage {
			var msg map[string]any
			if json.Unmarshal(data, &msg) == nil {
				if msg["type"] == "stream_reset" {
					sawReset = true
				} else if msg["type"] == "stream_init" && sawReset {
					sawInit = true
				}
			}
		} else if mt == websocket.BinaryMessage && sawInit {
			if isKeyAUPacket(data) {
				keyReceived = time.Now()
				break
			}
		}
	}
	keyframeRecovery := keyReceived.Sub(reqT0)

	// 5. ABR Bitrate Shift
	abrT0 := time.Now()
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"set_bitrate","kbps":1200}`)); err != nil {
		t.Fatalf("send set_bitrate: %v", err)
	}
	var abrReceived time.Time
	var sawAbrReset, sawAbrInit bool
	for time.Now().Before(abrT0.Add(3 * time.Second)) {
		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		mt, data, err := conn.ReadMessage()
		if err != nil {
			continue
		}
		if mt == websocket.TextMessage {
			var msg map[string]any
			if json.Unmarshal(data, &msg) == nil {
				if msg["type"] == "stream_reset" {
					sawAbrReset = true
				} else if msg["type"] == "stream_init" && sawAbrReset {
					sawAbrInit = true
				}
			}
		} else if mt == websocket.BinaryMessage && sawAbrInit {
			abrReceived = time.Now()
			break
		}
	}
	abrSwitchTime := abrReceived.Sub(abrT0)

	fmt.Println("\n================ LATENCY & USER EXPERIENCE REPORT ================")
	fmt.Printf("1. Stream Init Handshake:                %v (mode=%v, codec=%v)\n", handshakeLatency, initMsg["mode"], initMsg["codec"])
	fmt.Printf("2. Time-to-First-Packet (TTFP):           %v\n", ttfp)
	fmt.Printf("3. Control Channel Ping/Pong (RTT):       %v\n", rtt)
	fmt.Printf("4. Interaction-to-Stream Edge Latency:    %v\n", changeLatency)
	fmt.Printf("5. On-Demand Keyframe Recovery Time:      %v\n", keyframeRecovery)
	fmt.Printf("6. ABR Dynamic Bitrate Switching Time:    %v\n", abrSwitchTime)
	fmt.Println("==================================================================")
}

func TestUXMseLatencyBenchmark(t *testing.T) {
	const port = "15931"
	os.Setenv("ISOLA_STATIC_KEEPALIVE_MS", "2000")
	listener := startTestStreamer(t, port, 2000)
	defer listener.Close()

	uds, err := net.Dial("unix", e2eTestSockPath)
	if err != nil {
		t.Fatalf("UDS dial: %v", err)
	}
	defer uds.Close()

	w, h := e2eTestWidth, e2eTestHeight
	plane := make([]byte, w*h*4)
	for i := 0; i < len(plane); i += 4 {
		plane[i] = 128
		plane[i+1] = 128
		plane[i+2] = 128
		plane[i+3] = 255
	}
	if err := writeFrame(uds, w, h, plane); err != nil {
		t.Fatalf("send initial frame: %v", err)
	}

	conn, initMsg := dialTestViewer(t, port)
	defer conn.Close()

	// Consume initial video segments
	for i := 0; i < 2; i++ {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		mt, _, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read init msg %d: %v", i, err)
		}
		if mt == websocket.TextMessage {
			i-- // don't count text metrics
		}
	}

	// Quiet pause so static suppression settles
	time.Sleep(200 * time.Millisecond)

	// Mutate surface (user interaction on a static page)
	for i := 0; i < len(plane); i += 4 {
		plane[i] = 0
		plane[i+1] = 255
		plane[i+2] = 0
		plane[i+3] = 255
	}
	changeT0 := time.Now()
	if err := writeFrame(uds, w, h, plane); err != nil {
		t.Fatalf("send change frame: %v", err)
	}

	var changeReceived time.Time
	for i := 0; i < 30; i++ {
		_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		mt, data, err := conn.ReadMessage()
		if err != nil {
			continue
		}
		if mt == websocket.BinaryMessage && len(data) > 0 {
			changeReceived = time.Now()
			break
		}
	}
	if changeReceived.IsZero() {
		t.Fatal("no binary fragment received after surface change within 3s")
	}

	mseChangeLatency := changeReceived.Sub(changeT0)
	fmt.Println("\n================ MSE (fMP4) LATENCY & FLUSH REPORT ================")
	fmt.Printf("MSE Mode Codec: %v\n", initMsg["codec"])
	fmt.Printf("Quiet Page Single Interaction Delivery: %v\n", mseChangeLatency)
	if mseChangeLatency < 350*time.Millisecond {
		fmt.Printf("Verdict: EXCELLENT (Flushed via 40ms follow-up in %v, well below 2000ms keepalive!)\n", mseChangeLatency)
	} else {
		fmt.Printf("Verdict: SLOW (took %v)\n", mseChangeLatency)
	}
	fmt.Println("==================================================================")
}
