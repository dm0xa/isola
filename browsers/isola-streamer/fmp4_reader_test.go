package main

import (
	"bytes"
	"testing"
)

func TestFMP4ReaderInitAndFragments(t *testing.T) {
	r := newFMP4Reader()

	ftyp := makeBox("ftyp", 20)
	moov := makeBox("moov", 100)
	init := append(ftyp, moov...)
	moof1 := makeBox("moof", 50)
	mdat1 := makeBox("mdat", 200)
	frag1 := append(moof1, mdat1...)

	raw := append(init, frag1...)

	// Split init across ftyp / moov reads (the bug we saw live).
	segs := r.Feed(ftyp)
	if len(segs) != 0 {
		t.Fatalf("expected no segments after partial ftyp, got %d", len(segs))
	}
	segs = r.Feed(moov)
	if len(segs) != 1 || len(segs[0]) != len(init) {
		t.Fatalf("expected one init segment len=%d, got %d segs", len(init), len(segs))
	}

	segs = r.Feed(frag1[:100])
	if len(segs) != 0 {
		t.Fatalf("expected no segments from partial fragment")
	}
	segs = r.Feed(frag1[100:])
	if len(segs) != 1 || len(segs[0]) != len(frag1) {
		t.Fatalf("expected one media fragment len=%d, got %d", len(frag1), len(segs))
	}

	// Arbitrary 64 KiB splits like pipe reads.
	r2 := newFMP4Reader()
	var all [][]byte
	chunk := 65536
	for i := 0; i < len(raw); i += chunk {
		end := i + chunk
		if end > len(raw) {
			end = len(raw)
		}
		all = append(all, r2.Feed(raw[i:end])...)
	}
	if len(all) != 2 {
		t.Fatalf("expected init + 1 fragment, got %d segments", len(all))
	}
	if !bytes.Equal(all[0], init) {
		t.Fatal("init segment mismatch")
	}
	if !bytes.Equal(all[1], frag1) {
		t.Fatal("media fragment mismatch")
	}
}

func makeBox(typ string, contentLen int) []byte {
	size := 8 + contentLen
	b := make([]byte, size)
	b[4], b[5], b[6], b[7] = typ[0], typ[1], typ[2], typ[3]
	b[0] = byte(size >> 24)
	b[1] = byte(size >> 16)
	b[2] = byte(size >> 8)
	b[3] = byte(size)
	return b
}
