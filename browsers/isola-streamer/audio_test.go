package main

import (
	"bytes"
	"testing"
)

// TestRtpPayloadRejectsRTCP guards the RFC 5761 single-port demux: ffmpeg's
// RTP muxer writes RTCP Sender Reports to the same socket as the Opus RTP
// stream, and RTCP also carries version 2 in the first octet. Forwarding it
// as an Opus packet makes the viewer's AudioDecoder throw
// "EncodingError: Decoding error".
func TestRtpPayloadRejectsRTCP(t *testing.T) {
	// Real first datagram captured from the container's ffmpeg:
	// V=2 (0x80), PT=200 (RTCP SR, second octet 0xC8), length 28 bytes.
	sr := []byte{
		0x80, 0xc8, 0x00, 0x06, 0xc7, 0x5f, 0xaa, 0x6b,
		0xee, 0x4c, 0xec, 0x35, 0xfc, 0x6a, 0x7e, 0xf9,
		0x92, 0xde, 0x10, 0xd3, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}
	if _, ok := rtpPayload(sr); ok {
		t.Fatal("rtpPayload accepted an RTCP Sender Report")
	}

	// Every RTCP packet type (200–207) must be rejected, with and without
	// the marker bit set.
	for pt := 200; pt <= 207; pt++ {
		for _, mbit := range []byte{0x00, 0x80} {
			pkt := make([]byte, 12)
			pkt[0] = 0x80
			pkt[1] = byte(pt) | mbit
			if _, ok := rtpPayload(pkt); ok {
				t.Fatalf("rtpPayload accepted RTCP type %d (marker %#x)", pt, mbit)
			}
		}
	}
}

// TestRtpPayloadAcceptsOpus checks that a genuine ffmpeg Opus RTP packet
// (dynamic PT 97) still yields its payload.
func TestRtpPayloadAcceptsOpus(t *testing.T) {
	// V=2, M=0, PT=97 (0x61), seq 0x07b5, ts, ssrc, then 20 bytes payload.
	pkt := make([]byte, 12+20)
	pkt[0] = 0x80
	pkt[1] = 0x61
	copy(pkt[12:], bytes.Repeat([]byte{0xAA}, 20))

	payload, ok := rtpPayload(pkt)
	if !ok {
		t.Fatal("rtpPayload rejected a valid Opus RTP packet")
	}
	if len(payload) != 20 || payload[0] != 0xAA {
		t.Fatalf("rtpPayload returned %d bytes (%#x), want 20 bytes of 0xAA", len(payload), payload[0])
	}
}
