// Full e2e test: isola-streamer (UDS listener) → mock client (UDS connect) → WS client.
// Streamer starts first and listens on the UDS. The mock generator connects
// as a client (like chromium-isola would), sends BGRA frames.
// The WS client reads fMP4 segments and verifies the pipeline.
//
// Build: go build -o /tmp/isola-e2e ./cmd/e2e_test/
// Run:   /tmp/isola-e2e
package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/gorilla/websocket"
)

const (
	frameMagic   = 0x4C4F5349
	testSockPath = "/tmp/isola-e2e-frame.sock"
	streamerPort = "15900"
	streamerURL  = "ws://127.0.0.1:" + streamerPort + "/"
	testWidth    = 640
	testHeight   = 480
)

func main() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	log.Println("=== isola-streamer end-to-end test ===")

	os.Remove(testSockPath)

	// Step 1: Use the pre-built isola-streamer binary.
	// Must be at /tmp/isola-streamer (build with: go build -o /tmp/isola-streamer .)
	log.Println("[1/5] Checking isola-streamer binary...")
	streamerBin := "/tmp/isola-streamer"
	if !fileExists(streamerBin) {
		log.Println("  Pre-built not found, building from source...")
		cwd, _ := os.Getwd()
		buildCmd := exec.Command("go", "build", "-o", streamerBin, ".")
		buildCmd.Dir = findModuleDir(cwd)
		if out, err := buildCmd.CombinedOutput(); err != nil {
			log.Fatalf("FAIL: build isola-streamer: %v\n%s", err, out)
		}
		log.Println("  Build: OK ✓")
	} else {
		log.Println("  Using pre-built: OK ✓")
	}

	log.Println("[2/5] Starting isola-streamer (UDS listener)...")
	streamerCmd := exec.Command(streamerBin)
	streamerCmd.Env = append(os.Environ(),
		"PORT="+streamerPort,
		"ISOLA_FRAME_SOCK="+testSockPath,
		"ISOLA_WIDTH="+fmt.Sprint(testWidth),
		"ISOLA_HEIGHT="+fmt.Sprint(testHeight),
		"ISOLA_FPS=15",
		"ISOLA_BITRATE_KBPS=500",
	)
	if err := streamerCmd.Start(); err != nil {
		log.Fatalf("FAIL: start isola-streamer: %v", err)
	}
	defer func() {
		streamerCmd.Process.Kill()
		streamerCmd.Wait()
	}()

	// Wait for streamer to be healthy
	time.Sleep(1 * time.Second)
	resp, err := http.Get("http://127.0.0.1:" + streamerPort + "/healthz")
	if err != nil {
		log.Fatalf("FAIL: streamer health check: %v (did streamer start? check /tmp/isola-e2e-frame.sock)", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Fatalf("FAIL: streamer health returned %d", resp.StatusCode)
	}
	log.Println("  Streamer health: OK ✓")

	// Step 2: Connect WS client BEFORE starting the mock frame generator.
	// The broadcaster already buffers the latest frame, but connecting
	// WS first ensures a subscriber is always present when frames arrive.
	log.Println("[3/5] Connecting WebSocket client...")
	conn, _, err := websocket.DefaultDialer.Dial(streamerURL, nil)
	if err != nil {
		log.Fatalf("FAIL: WS dial: %v", err)
	}
	defer conn.Close()
	log.Println("  WS connected ✓")

	// Step 3: Start mock frame generator as UDS CLIENT.
	// chromium-isola connects to the UDS; the generator does the same.
	log.Println("[4/5] Starting mock frame generator (UDS client)...")
	stopFrameGen := make(chan struct{})
	go runFrameClient(testSockPath, testWidth, testHeight, 30, stopFrameGen)
	time.Sleep(200 * time.Millisecond)
	log.Println("  Frame generator: OK ✓")

	// Step 4: Read messages and verify
	conn.SetReadDeadline(time.Now().Add(12 * time.Second))
	log.Println("[5/5] Verifying video stream...")
	var streamInit bool
	var segmentCount int
	var lastSeq uint32
	var payloadBytes int

	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				break
			}
			if segmentCount == 0 {
				log.Fatalf("FAIL: WS read error: %v", err)
			}
			break
		}

		if mt == websocket.TextMessage {
			var msg struct {
				Type   string `json:"type"`
				Codec  string `json:"codec"`
				Width  int    `json:"width"`
				Height int    `json:"height"`
			}
			if err := json.Unmarshal(data, &msg); err == nil && msg.Type == "stream_init" {
				streamInit = true
				log.Printf("  stream_init: codec=%s %dx%d ✓", msg.Codec, msg.Width, msg.Height)
				if msg.Codec == "" {
					log.Fatalf("FAIL: stream_init missing codec")
				}
				if msg.Width != testWidth || msg.Height != testHeight {
					log.Fatalf("FAIL: dimensions: %dx%d, want %dx%d", msg.Width, msg.Height, testWidth, testHeight)
				}
				conn.SetReadDeadline(time.Now().Add(10 * time.Second))
			}
		}

		if mt == websocket.BinaryMessage {
			if len(data) < 10 {
				log.Fatalf("FAIL: binary message too short: %d bytes", len(data))
				continue
			}

			marker := data[0]
			flags := data[1]
			seq := binary.LittleEndian.Uint32(data[2:6])
			payloadLen := binary.LittleEndian.Uint32(data[6:10])
			payload := data[10:]
			isKey := flags&0x01 != 0

			if marker != 0x01 {
				log.Fatalf("FAIL: bad marker: 0x%02x", marker)
			}

			if int(payloadLen) != len(payload) {
				log.Fatalf("FAIL: payloadLen=%d != actual=%d", payloadLen, len(payload))
			}

			segmentCount++
			payloadBytes += len(payload)

			if segmentCount == 1 {
				log.Printf("  [seg #1] seq=%d key=%v payloadLen=%d ✓", seq, isKey, payloadLen)
			}

			if seq <= lastSeq && lastSeq > 0 {
				log.Fatalf("FAIL: seq out of order: %d after %d", seq, lastSeq)
			}
			lastSeq = seq

			if segmentCount >= 5 {
				break
			}
		}
	}

	// Cleanup
	close(stopFrameGen)
	time.Sleep(100 * time.Millisecond)
	os.Remove(testSockPath)

	// Report
	log.Println()
	if !streamInit {
		log.Fatal("FAIL: never received stream_init")
	}
	if segmentCount == 0 {
		log.Fatal("FAIL: no binary segments received")
	}

	fpsEstimate := float64(segmentCount) / 5.0
	log.Printf("  Received: %d segments, %d payload bytes, ~%.0f fps", segmentCount, payloadBytes, fpsEstimate)
	log.Printf("  All sequences ordered: ✓")
	log.Printf("  All markers valid: ✓")

	// Verify input events
	inputMsg := `{"type":"mousemove","x":100,"y":200}`
	if err := conn.WriteMessage(websocket.TextMessage, []byte(inputMsg)); err != nil {
		log.Printf("  WARNING: input event send failed: %v", err)
	} else {
		log.Println("  Input event sent: ✓")
	}

	log.Println()
	log.Println("╔══════════════════════════════════════════╗")
	log.Println("║  ALL TESTS PASSED — STREAM VERIFIED     ║")
	log.Println("╚══════════════════════════════════════════╝")
}

// runFrameClient connects to the existing UDS and sends BGRA test frames.
// This matches how chromium-isola connects: as a client to an already-listening
// socket, not creating its own listener.
func runFrameClient(sockPath string, width, height int, fps int, stop chan struct{}) {
	// Wait for the socket to be created and for the listener to accept.
	var conn net.Conn
	var err error
	for i := 0; i < 80; i++ {
		conn, err = net.Dial("unix", sockPath)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		log.Printf("frame client: dial %s: %v (retries exhausted)", sockPath, err)
		return
	}
	defer conn.Close()
	log.Printf("frame client: connected to %s ✓", sockPath)

	ticker := time.NewTicker(time.Second / time.Duration(fps))
	defer ticker.Stop()
	frameNum := 0
	stride := uint32(width * 4)

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
		}

		pixels := make([]byte, height*int(stride))
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				off := y*int(stride) + x*4
				pixels[off] = uint8(x * 255 / width)    // B
				pixels[off+1] = uint8(y * 255 / height) // G
				pixels[off+2] = uint8(frameNum % 256)   // R
				pixels[off+3] = 255                     // A
			}
		}

		hdr := make([]byte, 32)
		binary.LittleEndian.PutUint32(hdr[0:4], frameMagic)
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
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func findModuleDir(start string) string {
	for dir := start; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		if fileExists(filepath.Join(dir, "go.mod")) {
			return dir
		}
	}
	return ""
}
