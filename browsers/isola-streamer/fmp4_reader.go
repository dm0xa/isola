package main

import "encoding/binary"

// fmp4Reader reassembles ffmpeg fragmented-MP4 output into MSE-ready segments:
//   - init: ftyp + moov (one append)
//   - media: moof + mdat (one append per fragment)
type fmp4Reader struct {
	buf      []byte
	initSent bool
}

func newFMP4Reader() *fmp4Reader {
	return &fmp4Reader{}
}

// Feed appends raw ffmpeg stdout bytes and returns any complete segments.
func (r *fmp4Reader) Feed(data []byte) [][]byte {
	if len(data) == 0 {
		return nil
	}
	r.buf = append(r.buf, data...)
	var out [][]byte
	for {
		seg, ok := r.extract()
		if !ok {
			break
		}
		out = append(out, seg)
	}
	return out
}

func (r *fmp4Reader) extract() ([]byte, bool) {
	for len(r.buf) >= 8 {
		if !r.initSent {
			if string(r.buf[4:8]) != "ftyp" {
				// Drop bytes until ftyp (shouldn't happen with ffmpeg).
				r.buf = r.buf[1:]
				continue
			}
			ftypSize, ok := boxSize(r.buf, 0)
			if !ok || len(r.buf) < ftypSize+8 {
				return nil, false
			}
			moovOff := ftypSize
			if string(r.buf[moovOff+4:moovOff+8]) != "moov" {
				return nil, false
			}
			moovSize, ok := boxSize(r.buf, moovOff)
			if !ok {
				return nil, false
			}
			initEnd := moovOff + moovSize
			if len(r.buf) < initEnd {
				return nil, false
			}
			seg := make([]byte, initEnd)
			copy(seg, r.buf[:initEnd])
			r.buf = r.buf[initEnd:]
			r.initSent = true
			return seg, true
		}

		if string(r.buf[4:8]) != "moof" {
			// Skip stray boxes between fragments.
			size, ok := boxSize(r.buf, 0)
			if !ok || len(r.buf) < size {
				return nil, false
			}
			r.buf = r.buf[size:]
			continue
		}
		moofSize, ok := boxSize(r.buf, 0)
		if !ok || len(r.buf) < moofSize+8 {
			return nil, false
		}
		mdatOff := moofSize
		if string(r.buf[mdatOff+4:mdatOff+8]) != "mdat" {
			return nil, false
		}
		mdatSize, ok := boxSize(r.buf, mdatOff)
		if !ok {
			return nil, false
		}
		fragEnd := mdatOff + mdatSize
		if len(r.buf) < fragEnd {
			return nil, false
		}
		seg := make([]byte, fragEnd)
		copy(seg, r.buf[:fragEnd])
		r.buf = r.buf[fragEnd:]
		return seg, true
	}
	return nil, false
}

func boxSize(data []byte, off int) (int, bool) {
	if off+8 > len(data) {
		return 0, false
	}
	size32 := binary.BigEndian.Uint32(data[off : off+4])
	if size32 == 0 {
		return 0, false
	}
	if size32 == 1 {
		// 64-bit extended size — not expected from our ffmpeg settings.
		if off+16 > len(data) {
			return 0, false
		}
		size64 := binary.BigEndian.Uint64(data[off+8 : off+16])
		if size64 < 16 || size64 > 1<<30 {
			return 0, false
		}
		return int(size64), true
	}
	if size32 < 8 {
		return 0, false
	}
	return int(size32), true
}
