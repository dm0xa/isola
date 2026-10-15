package main

import (
	"bytes"
	"testing"
)

// surface builds a uniform BGRA buffer.
func surface(stride, height uint32, b, g, r byte) []byte {
	buf := make([]byte, int(stride)*int(height))
	for i := 0; i < len(buf); i += 4 {
		buf[i], buf[i+1], buf[i+2], buf[i+3] = b, g, r, 0xFF
	}
	return buf
}

func TestStampWatermarkDisabled(t *testing.T) {
	old := watermarkText
	watermarkText = ""
	defer func() { watermarkText = old }()

	stride, height := uint32(64*4), uint32(32)
	buf := surface(stride, height, 10, 20, 30)
	stampWatermark(buf, stride, height)
	if !bytes.Equal(buf, surface(stride, height, 10, 20, 30)) {
		t.Fatal("disabled watermark must not touch pixels")
	}
}

func TestStampWatermarkChangesBottomRight(t *testing.T) {
	old := watermarkText
	watermarkText = "user@corp"
	defer func() { watermarkText = old }()

	stride, height := uint32(320*4), uint32(180)
	buf := surface(stride, height, 10, 20, 30)
	stampWatermark(buf, stride, height)

	changed, untouchedTop := 0, true
	total := int(stride) * int(height)
	for i := 0; i < total; i += 4 {
		if buf[i] != 10 || buf[i+1] != 20 || buf[i+2] != 30 {
			changed++
			if i < total/2 {
				untouchedTop = false
			}
		}
	}
	if changed == 0 {
		t.Fatal("watermark changed no pixels")
	}
	if !untouchedTop {
		t.Fatal("watermark bled into the top half")
	}
	// Alpha blend must keep the mark translucent: blended pixels stay close
	// to the background, not solid white.
	if buf[total-4+0] == 255 && buf[total-4+1] == 255 {
		t.Fatal("expected translucent blend, found solid white")
	}
}

func TestStampWatermarkSmallSurfaceNoCorrupt(t *testing.T) {
	old := watermarkText
	watermarkText = "verylongusername@example.com"
	defer func() { watermarkText = old }()

	// Surface far too small for the mark: must be a clean no-op.
	stride, height := uint32(20*4), uint32(10)
	buf := surface(stride, height, 1, 2, 3)
	stampWatermark(buf, stride, height)
	if !bytes.Equal(buf, surface(stride, height, 1, 2, 3)) {
		t.Fatal("small surface must be skipped, not corrupted")
	}
}

func TestStampWatermarkIdempotentBounds(t *testing.T) {
	old := watermarkText
	watermarkText = "sess 1234"
	defer func() { watermarkText = old }()

	stride, height := uint32(200*4), uint32(120)
	buf := surface(stride, height, 0, 0, 0)
	stampWatermark(buf, stride, height)
	// Applying to an already-stamped buffer must not panic (shadow/glyph
	// overlaps) — run again.
	stampWatermark(buf, stride, height)
}
