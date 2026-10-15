package main

// auSplitter reassembles an Annex-B H.264 byte stream (as produced by
// ffmpeg -f h264) into complete access units. An access unit boundary sits
// immediately before the second VCL NAL (type 1-5) of a run: non-VCL NALs
// (SPS/PPS/SEI) that arrive between two pictures belong to the *following*
// picture, so the emitted AU always carries its parameter sets with it —
// exactly what a WebCodecs VideoDecoder consuming Annex-B expects.
type auSplitter struct {
	buf []byte
}

type auPacket struct {
	data     []byte // complete AU, start codes included
	keyframe bool   // AU contains an IDR NAL (type 5)
}

// feed appends encoded bytes and returns every access unit that completed.
func (s *auSplitter) feed(data []byte) []auPacket {
	s.buf = append(s.buf, data...)
	var out []auPacket
	for {
		boundary, ok := s.findBoundary()
		if !ok {
			return out
		}
		au := make([]byte, boundary)
		copy(au, s.buf[:boundary])
		out = append(out, auPacket{data: au, keyframe: auContainsIDR(au)})
		// Compact in place — the tail is the next (incomplete) AU.
		s.buf = append(s.buf[:0], s.buf[boundary:]...)
	}
}

// findBoundary returns the offset at which the next access unit starts.
// With x264 zerolatency (sliced threads) a picture arrives as several slice
// NALs, so "first VCL NAL" is not enough: a new AU begins at a VCL NAL whose
// first_mb_in_slice == 0. first_mb_in_slice is the first ue(v) field of the
// slice header — it is 0 exactly when the first payload bit after the NAL
// header is 1, i.e. (buf[nal+1] & 0x80) != 0.
func (s *auSplitter) findBoundary() (int, bool) {
	firstVCL := -1
	off := 0
	for {
		startCode, nal, ok := nextNAL(s.buf, off)
		if !ok {
			return 0, false
		}
		t := int(s.buf[nal] & 0x1F)
		if t >= 1 && t <= 5 && nal+1 < len(s.buf) {
			firstSliceOfPicture := s.buf[nal+1]&0x80 != 0
			if firstSliceOfPicture {
				if firstVCL < 0 {
					firstVCL = startCode
				} else {
					return startCode, true
				}
			}
		}
		off = nal
	}
}

// nextNAL finds the next NAL boundary at or after `from`. It returns the
// offset of the start code (3- or 4-byte form) and the offset of the NAL
// header byte that follows it.
func nextNAL(buf []byte, from int) (startCode int, nalHeader int, ok bool) {
	i := from
	for i+3 <= len(buf) {
		if buf[i] == 0 && buf[i+1] == 0 && buf[i+2] == 1 {
			start := i
			if start > 0 && buf[start-1] == 0 {
				start-- // 4-byte start code 00 00 00 01
			}
			return start, i + 3, true
		}
		i++
	}
	return 0, 0, false
}

// auContainsIDR scans an access unit for an IDR NAL (type 5).
func auContainsIDR(au []byte) bool {
	off := 0
	for {
		_, nal, ok := nextNAL(au, off)
		if !ok {
			return false
		}
		if int(au[nal]&0x1F) == 5 {
			return true
		}
		off = nal
	}
}
