// mock-isola-frames: standalone UDS client that writes BGRA test frames.
// Connects to the isola-streamer's frame socket (like chromium-isola does)
// and emits a color gradient animation with frame counter.
//
// Usage: mock-isola-frames /run/isola/frame.sock [width=1280] [height=720] [fps=15]
package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

const frameMagic = 0x4C4F5349

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <uds_path> [width] [height] [fps]\n", os.Args[0])
		os.Exit(1)
	}

	sockPath := os.Args[1]
	width := 1280
	height := 720
	fps := 15

	if len(os.Args) >= 3 {
		w, err := strconv.Atoi(os.Args[2])
		if err == nil && w > 0 {
			width = w
		}
	}
	if len(os.Args) >= 4 {
		h, err := strconv.Atoi(os.Args[3])
		if err == nil && h > 0 {
			height = h
		}
	}
	if len(os.Args) >= 5 {
		f, err := strconv.Atoi(os.Args[4])
		if err == nil && f > 0 {
			fps = f
		}
	}

	log.Printf("mock-isola-frames: connecting to %s (%dx%d@%dfps)", sockPath, width, height, fps)

	// Wait for the streamer to create the socket
	var conn net.Conn
	var err error
	for i := 0; i < 100; i++ {
		conn, err = net.Dial("unix", sockPath)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		log.Fatalf("failed to connect to %s after retries: %v", sockPath, err)
	}
	defer conn.Close()
	log.Printf("connected to %s ✓", sockPath)

	// Shutdown on signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() { <-sigCh; conn.Close(); os.Exit(0) }()

	ticker := time.NewTicker(time.Second / time.Duration(fps))
	defer ticker.Stop()
	frameNum := 0
	stride := uint32(width * 4)

	for range ticker.C {
		pixels := make([]byte, height*int(stride))
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				off := y*int(stride) + x*4
				pixels[off] = uint8((x * 255) / width)    // B
				pixels[off+1] = uint8((y * 255) / height) // G
				pixels[off+2] = uint8(frameNum % 256)     // R
				pixels[off+3] = 255                       // A
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
