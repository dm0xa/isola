package main

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

// extractRows mirrors what the electron producer puts on the wire for one
// dirty rect: tightly packed rows, no stride padding.
func extractRows(surface []byte, stride uint32, r Rect) []byte {
	rowBytes := int(r.W) * 4
	out := make([]byte, 0, rowBytes*int(r.H))
	for row := 0; row < int(r.H); row++ {
		off := (int(r.Y)+row)*int(stride) + int(r.X)*4
		out = append(out, surface[off:off+rowBytes]...)
	}
	return out
}

// encodeTestFrame builds a wire frame carrying only the given rects' pixels.
func encodeTestFrame(w, h uint32, surface []byte, rects []Rect, tsNs uint64) []byte {
	stride := w * 4
	hdr := make([]byte, 32)
	binary.LittleEndian.PutUint32(hdr[0:4], frameMagic)
	binary.LittleEndian.PutUint32(hdr[4:8], w)
	binary.LittleEndian.PutUint32(hdr[8:12], h)
	binary.LittleEndian.PutUint32(hdr[12:16], stride)
	binary.LittleEndian.PutUint32(hdr[20:24], uint32(len(rects)))
	binary.LittleEndian.PutUint64(hdr[24:32], tsNs)

	buf := append([]byte(nil), hdr...)
	for _, r := range rects {
		rb := make([]byte, 16)
		binary.LittleEndian.PutUint32(rb[0:4], r.X)
		binary.LittleEndian.PutUint32(rb[4:8], r.Y)
		binary.LittleEndian.PutUint32(rb[8:12], r.W)
		binary.LittleEndian.PutUint32(rb[12:16], r.H)
		buf = append(buf, rb...)
	}
	for _, r := range rects {
		buf = append(buf, extractRows(surface, stride, r)...)
	}
	return buf
}

// rawWireFrame builds a header + rect table with an arbitrary payload length.
// Used for malformed frames, where extracting real pixels would be invalid.
func rawWireFrame(w, h uint32, rects []Rect, payloadLen int) []byte {
	hdr := make([]byte, 32)
	binary.LittleEndian.PutUint32(hdr[0:4], frameMagic)
	binary.LittleEndian.PutUint32(hdr[4:8], w)
	binary.LittleEndian.PutUint32(hdr[8:12], h)
	binary.LittleEndian.PutUint32(hdr[12:16], w*4)
	binary.LittleEndian.PutUint32(hdr[20:24], uint32(len(rects)))

	buf := append([]byte(nil), hdr...)
	for _, r := range rects {
		rb := make([]byte, 16)
		binary.LittleEndian.PutUint32(rb[0:4], r.X)
		binary.LittleEndian.PutUint32(rb[4:8], r.Y)
		binary.LittleEndian.PutUint32(rb[8:12], r.W)
		binary.LittleEndian.PutUint32(rb[12:16], r.H)
		buf = append(buf, rb...)
	}
	return append(buf, make([]byte, payloadLen)...)
}

func testSurface(w, h uint32) []byte {
	s := make([]byte, int(w*h*4))
	for i := range s {
		s[i] = byte(i % 251)
	}
	return s
}

func readFrameFromBytes(t *testing.T, wire []byte) (*Frame, error) {
	t.Helper()
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	go func() { _, _ = client.Write(wire) }()
	return readFrame(server)
}

// TestReadFrameDamagePayload verifies the decoder reads only the damaged
// region off the wire and reports the rect that describes it.
func TestReadFrameDamagePayload(t *testing.T) {
	const w, h = 8, 4
	surface := testSurface(w, h)

	t.Run("full refresh", func(t *testing.T) {
		full := Rect{X: 0, Y: 0, W: w, H: h}
		f, err := readFrameFromBytes(t, encodeTestFrame(w, h, surface, []Rect{full}, 4242))
		if err != nil {
			t.Fatalf("readFrame: %v", err)
		}
		if f.Width != w || f.Height != h || f.Stride != w*4 {
			t.Fatalf("geometry = %dx%d stride=%d, want %dx%d stride=%d",
				f.Width, f.Height, f.Stride, w, h, w*4)
		}
		if !f.IsFull() {
			t.Error("a rect covering the whole surface must report IsFull")
		}
		if !bytes.Equal(f.Payload, surface) {
			t.Error("full refresh payload must be the whole surface")
		}
	})

	t.Run("damage update", func(t *testing.T) {
		r := Rect{X: 3, Y: 1, W: 3, H: 2}
		f, err := readFrameFromBytes(t, encodeTestFrame(w, h, surface, []Rect{r}, 7))
		if err != nil {
			t.Fatalf("readFrame: %v", err)
		}
		if len(f.Dirty) != 1 || f.Dirty[0] != r {
			t.Fatalf("rect = %+v, want %+v", f.Dirty, r)
		}
		if f.IsFull() {
			t.Error("a partial rect must not report IsFull")
		}
		if len(f.Payload) != int(r.W)*int(r.H)*4 {
			t.Fatalf("payload = %d bytes, want %d", len(f.Payload), int(r.W)*int(r.H)*4)
		}
		if !bytes.Equal(f.Payload, extractRows(surface, w*4, r)) {
			t.Error("payload does not match the damaged pixels")
		}
	})
}

// TestReadFrameRejectsOutOfBounds guards the stream from desynchronising: the
// payload length is derived from the rects, so a bogus rect must be refused
// before any pixels are read.
func TestReadFrameRejectsOutOfBounds(t *testing.T) {
	const w, h = 8, 4

	cases := []struct {
		name  string
		rects []Rect
	}{
		{"x overflow", []Rect{{X: 6, Y: 0, W: 4, H: 1}}},
		{"y overflow", []Rect{{X: 0, Y: 3, W: 1, H: 2}}},
		{"zero width", []Rect{{X: 0, Y: 0, W: 0, H: 1}}},
		{"no rects", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wire := rawWireFrame(w, h, tc.rects, 4*1024)
			if _, err := readFrameFromBytes(t, wire); err == nil {
				t.Fatalf("expected an error for %s", tc.name)
			}
		})
	}
}

func waitForSurface(t *testing.T, fb *frameBroadcaster, want []byte) {
	t.Helper()
	got := make([]byte, len(want))
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fb.Snapshot(got) && bytes.Equal(got, want) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("surface never matched the expected image")
}

// TestBroadcasterComposesDamage is the core guarantee of damage-only
// transport: a partial update must change only its own rect and leave the
// rest of the composed surface intact.
func TestBroadcasterComposesDamage(t *testing.T) {
	const w, h = 8, 4
	stride := uint32(w * 4)
	surface := testSurface(w, h)

	incoming := make(chan *Frame, 4)
	fb := newFrameBroadcaster(incoming)

	full := Rect{X: 0, Y: 0, W: w, H: h}
	seed := &Frame{
		Width: w, Height: h, Stride: stride, Dirty: []Rect{full},
		Payload: extractRows(surface, stride, full),
	}
	if !seed.IsFull() {
		t.Fatal("expected a full-surface frame to report IsFull")
	}
	incoming <- seed
	waitForSurface(t, fb, surface)

	// A 2x1 damage rect at (3,1).
	damage := Rect{X: 3, Y: 1, W: 2, H: 1}
	if (&Frame{Dirty: []Rect{damage}}).IsFull() {
		t.Fatal("a partial rect must not report IsFull")
	}
	for i := 0; i < 8; i++ { // 2 pixels × 4 bytes
		surface[int(damage.Y)*int(stride)+int(damage.X)*4+i] = 0xAB
	}
	incoming <- &Frame{
		Width: w, Height: h, Stride: stride, Dirty: []Rect{damage},
		Payload: extractRows(surface, stride, damage),
	}
	waitForSurface(t, fb, surface)

	// Nothing outside the damage rect may have changed.
	got := make([]byte, len(surface))
	if !fb.Snapshot(got) {
		t.Fatal("no surface available")
	}
	if !bytes.Equal(got, surface) {
		t.Fatal("composed surface does not match the expected image after damage")
	}
}

// TestChangeSignal verifies the signal the encode loop relies on to suppress
// static frames: re-applying identical pixels reports no change and leaves the
// broadcaster's change counter still, while a differing payload reports a
// change and advances it.
func TestChangeSignal(t *testing.T) {
	const w, h = 8, 4
	stride := uint32(w * 4)
	surface := testSurface(w, h)
	rect := Rect{X: 2, Y: 1, W: 3, H: 2}
	payload := extractRows(surface, stride, rect)

	t.Run("applyRects reports pixel differences", func(t *testing.T) {
		dst := append([]byte(nil), surface...)
		if applyRects(dst, stride, &Frame{Dirty: []Rect{rect}, Payload: payload}) {
			t.Error("re-applying identical pixels must report no change")
		}
		diff := append([]byte(nil), payload...)
		diff[0] ^= 0xFF
		if !applyRects(dst, stride, &Frame{Dirty: []Rect{rect}, Payload: diff}) {
			t.Error("differing pixels must report a change")
		}
	})

	t.Run("composeLocked advances the counter only on change", func(t *testing.T) {
		fb := &frameBroadcaster{}
		if seq, at := fb.ChangeState(); seq != 0 || !at.IsZero() {
			t.Fatalf("empty broadcaster reported seq=%d at=%v, want 0/zero", seq, at)
		}

		full := &Frame{Width: w, Height: h, Stride: stride,
			Dirty: []Rect{{X: 0, Y: 0, W: w, H: h}}, Payload: surface}
		fb.mu.Lock()
		fb.composeLocked(full)
		seedSeq := fb.changeSeq
		fb.mu.Unlock()
		if seedSeq == 0 {
			t.Fatal("seeding the surface must register a change")
		}

		identical := &Frame{Width: w, Height: h, Stride: stride,
			Dirty: []Rect{rect}, Payload: payload}
		fb.mu.Lock()
		fb.composeLocked(identical)
		sameSeq := fb.changeSeq
		fb.mu.Unlock()
		if sameSeq != seedSeq {
			t.Fatalf("identical repaint advanced the counter: %d -> %d", seedSeq, sameSeq)
		}

		diff := append([]byte(nil), payload...)
		diff[0] ^= 0xFF
		fb.mu.Lock()
		fb.composeLocked(&Frame{Width: w, Height: h, Stride: stride,
			Dirty: []Rect{rect}, Payload: diff})
		changedSeq := fb.changeSeq
		fb.mu.Unlock()
		if changedSeq == sameSeq {
			t.Fatalf("differing pixels did not advance the counter (still %d)", changedSeq)
		}

		// A geometry change reallocates the surface and must count as a change
		// even when the incoming pixels happen to be identical.
		fb.mu.Lock()
		fb.composeLocked(&Frame{Width: w, Height: h * 2, Stride: stride,
			Dirty:   []Rect{{X: 0, Y: 0, W: w, H: h * 2}},
			Payload: make([]byte, int(stride)*h*2)})
		geomSeq := fb.changeSeq
		fb.mu.Unlock()
		if geomSeq == changedSeq {
			t.Fatalf("geometry change did not advance the counter (still %d)", geomSeq)
		}
	})
}

// TestDamageCounters pins the capture-damage measurement that decides whether a
// region-based encoder is worth building: how much of the surface each applied
// update actually touches (summed rects vs. the single bounding box a
// coalescing encoder would ship), and that an update which changes no pixel is
// not counted — it never reaches an encoder, so counting it would understate
// the damage fraction of the frames that do.
func TestDamageCounters(t *testing.T) {
	const w, h = 8, 4
	stride := uint32(w * 4)
	surface := testSurface(w, h)
	full := Rect{X: 0, Y: 0, W: w, H: h}
	seed := func(fb *frameBroadcaster) {
		fb.mu.Lock()
		defer fb.mu.Unlock()
		fb.composeLocked(&Frame{Width: w, Height: h, Stride: stride,
			Dirty: []Rect{full}, Payload: extractRows(surface, stride, full)})
	}

	t.Run("a full-surface frame counts as whole-image damage", func(t *testing.T) {
		fb := &frameBroadcaster{}
		seed(fb)

		fb.mu.RLock()
		dc := fb.damage
		fb.mu.RUnlock()

		if dc.Updates != 1 || dc.FullFrames != 1 {
			t.Fatalf("updates=%d full_frames=%d, want 1/1", dc.Updates, dc.FullFrames)
		}
		want := uint64(w * h)
		if dc.DamagedPx != want || dc.BBoxPx != want || dc.SurfacePx != want {
			t.Fatalf("damaged=%d bbox=%d surface=%d, want %d each",
				dc.DamagedPx, dc.BBoxPx, dc.SurfacePx, want)
		}
	})

	t.Run("an update that changes no pixel is not counted", func(t *testing.T) {
		fb := &frameBroadcaster{}
		frame := &Frame{Width: w, Height: h, Stride: stride,
			Dirty: []Rect{full}, Payload: extractRows(surface, stride, full)}

		fb.mu.Lock()
		fb.composeLocked(frame)
		fb.composeLocked(frame) // identical repaint: suppressed, never encoded
		dc := fb.damage
		fb.mu.Unlock()

		if dc.Updates != 1 {
			t.Fatalf("Updates = %d after an identical repaint, want 1", dc.Updates)
		}
	})

	t.Run("rects are summed but the bounding box is what coalescing pays", func(t *testing.T) {
		fb := &frameBroadcaster{}
		seed(fb)

		// Two 1x1 corners, top-left and bottom-right. Payloads are the surface
		// bytes with one bit flipped so the damage is a real pixel difference.
		a := Rect{X: 0, Y: 0, W: 1, H: 1}
		b := Rect{X: w - 1, Y: h - 1, W: 1, H: 1}
		pa := extractRows(surface, stride, a)
		pa[0] ^= 0xFF
		pb := extractRows(surface, stride, b)
		pb[0] ^= 0xFF

		fb.mu.Lock()
		fb.composeLocked(&Frame{Width: w, Height: h, Stride: stride,
			Dirty: []Rect{a, b}, Payload: append(pa, pb...)})
		dc := fb.damage
		fb.mu.Unlock()

		if dc.Updates != 2 {
			t.Fatalf("Updates = %d, want 2", dc.Updates)
		}
		if dc.FullFrames != 1 {
			t.Errorf("FullFrames = %d, want 1 (only the seed was full-surface)", dc.FullFrames)
		}
		if want := uint64(w*h + 2); dc.DamagedPx != want {
			t.Errorf("DamagedPx = %d, want %d (sum over rects)", dc.DamagedPx, want)
		}
		// The box spanning both corners covers the whole surface, which is the
		// argument for shipping damage as separate rects rather than one rect.
		if want := uint64(w*h + w*h); dc.BBoxPx != want {
			t.Errorf("BBoxPx = %d, want %d (bounding box spans the surface)", dc.BBoxPx, want)
		}
		if want := uint64(2 * w * h); dc.SurfacePx != want {
			t.Errorf("SurfacePx = %d, want %d", dc.SurfacePx, want)
		}
	})
}

// TestDamageMeter pins the per-second view of the cumulative counters. This is
// the measurement that separates "the picture is changing" from "the encoder is
// being fed the same frame again": the totals cannot tell those apart, because
// a session static for a minute and one that went static a second ago have the
// same numbers.
func TestDamageMeter(t *testing.T) {
	t0 := time.Unix(1000, 0)

	t.Run("the first sample only sets the baseline", func(t *testing.T) {
		var m damageMeter
		got := m.sample(damageCounters{Updates: 9, SurfacePx: 900, DamagedPx: 90}, t0)
		if got.ChangesPerSec != 0 || got.FullPerSec != 0 || got.DamagedPct != 0 {
			t.Fatalf("first sample = %+v, want zero rates", got)
		}
	})

	t.Run("a static window reports no changes", func(t *testing.T) {
		var m damageMeter
		base := damageCounters{Updates: 5, FullFrames: 1, SurfacePx: 5 * 100, DamagedPx: 500}
		m.sample(base, t0)
		got := m.sample(base, t0.Add(time.Second))
		if got.ChangesPerSec != 0 || got.FullPerSec != 0 || got.DamagedPct != 0 {
			t.Fatalf("static window = %+v, want all zero (a frozen picture must not read as changing)", got)
		}
	})

	t.Run("rates scale by the window and damage is a mean share", func(t *testing.T) {
		var m damageMeter
		m.sample(damageCounters{Updates: 100, FullFrames: 10, SurfacePx: 100 * 1000, DamagedPx: 2000}, t0)
		cur := damageCounters{Updates: 120, FullFrames: 14, SurfacePx: 120 * 1000, DamagedPx: 8000}
		got := m.sample(cur, t0.Add(2*time.Second))
		// 20 updates and 4 full frames across a 2s window.
		if got.ChangesPerSec != 10 || got.FullPerSec != 2 {
			t.Fatalf("rates = (%d, %d), want (10, 2)", got.ChangesPerSec, got.FullPerSec)
		}
		// 6000 damaged pixels over the 20000 surface pixels of that window.
		if got.DamagedPct != 30 {
			t.Fatalf("DamagedPct = %v, want 30", got.DamagedPct)
		}
	})

	t.Run("a replaced broadcaster resets the counters without going negative", func(t *testing.T) {
		var m damageMeter
		m.sample(damageCounters{Updates: 500, FullFrames: 50, SurfacePx: 500 * 1000, DamagedPx: 50000}, t0)
		got := m.sample(damageCounters{Updates: 3, FullFrames: 1, SurfacePx: 3000, DamagedPx: 3000}, t0.Add(time.Second))
		if got.ChangesPerSec != 3 || got.FullPerSec != 1 {
			t.Fatalf("post-reset rates = (%d, %d), want (3, 1)", got.ChangesPerSec, got.FullPerSec)
		}
		if got.DamagedPct != 100 {
			t.Fatalf("post-reset DamagedPct = %v, want 100", got.DamagedPct)
		}
	})
}

// TestKeyframeReasonBookkeeping verifies the reason a run was started is
// reported for its first IDR and that every later keyframe reads "periodic",
// while keyframeAt advances each time.
func TestKeyframeReasonBookkeeping(t *testing.T) {
	er := &streamRun{pendingKeyframeReason: "requested"}

	first := time.Unix(1000, 0)
	er.recordKeyframe(first)
	if got := er.keyframeAt.Load(); got != first.UnixNano() {
		t.Fatalf("keyframeAt = %d, want %d", got, first.UnixNano())
	}
	if er.lastKeyframeReason != "requested" {
		t.Fatalf("first keyframe reason = %q, want requested", er.lastKeyframeReason)
	}
	if er.pendingKeyframeReason != "" {
		t.Fatalf("startup reason not consumed: %q", er.pendingKeyframeReason)
	}

	second := time.Unix(1005, 0)
	er.recordKeyframe(second)
	if er.lastKeyframeReason != "periodic" {
		t.Fatalf("second keyframe reason = %q, want periodic", er.lastKeyframeReason)
	}
	if got := er.keyframeAt.Load(); got != second.UnixNano() {
		t.Fatalf("keyframeAt did not advance: %d", got)
	}
}

// TestKeyframeStateAccessor verifies the metrics accessor reports no keyframe
// for a fresh run and the written timestamp plus resolved reason after one.
func TestKeyframeStateAccessor(t *testing.T) {
	er := &streamRun{pendingKeyframeReason: "requested"}

	if at, reason := er.keyframeState(); !at.IsZero() || reason != "" {
		t.Fatalf("fresh run keyframeState = (%v, %q), want (zero, \"\")", at, reason)
	}

	at := time.Unix(2000, 0)
	er.recordKeyframe(at)
	gotAt, gotReason := er.keyframeState()
	if !gotAt.Equal(at) {
		t.Fatalf("keyframeState time = %v, want %v", gotAt, at)
	}
	if gotReason != "requested" {
		t.Fatalf("keyframeState reason = %q, want requested", gotReason)
	}
}
