package dnsproxy

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ── helpers ──────────────────────────────────────────────────────────────

// testQuery builds a minimal, well-formed DNS query (header + one question).
func testQuery() []byte {
	q := []byte{
		0xab, 0xcd, // ID
		0x01, 0x00, // RD
		0x00, 0x01, // QDCOUNT
		0x00, 0x00, // ANCOUNT
		0x00, 0x00, // NSCOUNT
		0x00, 0x00, // ARCOUNT
	}
	name := []byte{3, 'w', 'w', 'w', 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 3, 'c', 'o', 'm', 0}
	q = append(q, name...)
	q = append(q, 0x00, 0x01, 0x00, 0x01) // QTYPE=A, QCLASS=IN
	return q
}

var fakeTag = []byte("FAKE-UPSTREAM")

// fakeResponse is what the stand-in upstream returns: the query with the QR bit
// set plus a tag that proves the answer came from the upstream and was
// forwarded verbatim.
func fakeResponse(query []byte) []byte {
	resp := make([]byte, len(query), len(query)+len(fakeTag))
	copy(resp, query)
	resp[2] |= 0x80
	return append(resp, fakeTag...)
}

// startFakeUpstream runs a plaintext UDP+TCP DNS responder on a loopback port.
func startFakeUpstream(t *testing.T) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", pc.LocalAddr().String())
	if err != nil {
		pc.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close(); ln.Close() })

	go func() {
		buf := make([]byte, maxMessage)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = pc.WriteTo(fakeResponse(buf[:n]), addr)
		}
	}()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(2 * time.Second))
				query, err := readFramed(c)
				if err != nil {
					return
				}
				_ = writeFramed(c, fakeResponse(query))
			}(conn)
		}
	}()
	return pc.LocalAddr().String()
}

// startProxy listens on an ephemeral loopback port and serves until cleanup.
func startProxy(t *testing.T, upstreams []Upstream) string {
	t.Helper()
	srv := New([]string{"127.0.0.1:0"}, upstreams)
	addrs, err := srv.Listen()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = srv.Serve(ctx) }()
	t.Cleanup(cancel)
	return addrs[0]
}

func udpExchange(t *testing.T, addr string, query []byte) []byte {
	t.Helper()
	conn, err := net.Dial("udp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := conn.Write(query); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, maxMessage)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	return buf[:n]
}

func tcpExchange(t *testing.T, addr string, query []byte) []byte {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if err := writeFramed(conn, query); err != nil {
		t.Fatal(err)
	}
	resp, err := readFramed(conn)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func selfSignedCert(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// ── parsing ──────────────────────────────────────────────────────────────

func TestParseUpstream(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		mode    Protocol
		want    string
		wantErr bool
	}{
		{name: "bare ip uses mode plain", raw: "1.1.1.1", mode: ProtoPlain, want: "plain://1.1.1.1:53"},
		{name: "bare ip under tls mode", raw: "1.1.1.1", mode: ProtoTLS, want: "tls://1.1.1.1:853"},
		{name: "bare ip under https mode", raw: "1.1.1.1", mode: ProtoHTTPS, want: "https://1.1.1.1/dns-query"},
		{name: "explicit plain scheme wins", raw: "plain://9.9.9.9:5353", mode: ProtoTLS, want: "plain://9.9.9.9:5353"},
		{name: "tls scheme with port", raw: "tls://1.1.1.1:853", mode: ProtoPlain, want: "tls://1.1.1.1:853"},
		{name: "tls hostname gets default port", raw: "tls://dns.google", mode: ProtoPlain, want: "tls://dns.google:853"},
		{name: "doh url preserved", raw: "https://dns.google/dns-query", mode: ProtoPlain, want: "https://dns.google/dns-query"},
		{name: "doh scheme without path gets default path", raw: "https://cloudflare-dns.com", mode: ProtoPlain, want: "https://cloudflare-dns.com/dns-query"},
		{name: "empty is rejected", raw: "", mode: ProtoPlain, wantErr: true},
		{name: "unknown scheme is rejected", raw: "quic://1.1.1.1", mode: ProtoPlain, wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			u, err := ParseUpstream(tt.raw, tt.mode)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseUpstream(%q, %q) = %v, want error", tt.raw, tt.mode, u)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseUpstream(%q, %q) unexpected error: %v", tt.raw, tt.mode, err)
			}
			if got := u.String(); got != tt.want {
				t.Errorf("ParseUpstream(%q, %q) = %q, want %q", tt.raw, tt.mode, got, tt.want)
			}
		})
	}
}

func TestNewServerForMode(t *testing.T) {
	t.Parallel()

	t.Run("tls derives dot endpoints", func(t *testing.T) {
		t.Parallel()
		srv, err := NewServerForMode("tls", []string{"1.1.1.1", "8.8.8.8"}, []string{"127.0.0.1:0"})
		if err != nil {
			t.Fatal(err)
		}
		if len(srv.upstreams) != 2 || srv.upstreams[0].proto != ProtoTLS {
			t.Fatalf("unexpected upstreams: %+v", srv.upstreams)
		}
	})

	t.Run("url entries are used as-is", func(t *testing.T) {
		t.Parallel()
		srv, err := NewServerForMode("tls", []string{"tls://dns.google:853"}, []string{"127.0.0.1:0"})
		if err != nil {
			t.Fatal(err)
		}
		if got := srv.upstreams[0].String(); got != "tls://dns.google:853" {
			t.Errorf("upstream = %q, want tls://dns.google:853", got)
		}
	})

	t.Run("url scheme overrides the mode", func(t *testing.T) {
		t.Parallel()
		srv, err := NewServerForMode("tls", []string{"https://dns.google/dns-query"}, []string{"127.0.0.1:0"})
		if err != nil {
			t.Fatal(err)
		}
		if got := srv.upstreams[0].String(); got != "https://dns.google/dns-query" {
			t.Errorf("upstream = %q, want https://dns.google/dns-query", got)
		}
	})

	t.Run("plain mode is rejected", func(t *testing.T) {
		t.Parallel()
		if _, err := NewServerForMode("plain", []string{"1.1.1.1"}, nil); err == nil {
			t.Error("NewServerForMode(plain) = nil error, want error")
		}
	})

	t.Run("no upstreams is rejected", func(t *testing.T) {
		t.Parallel()
		if _, err := NewServerForMode("https", nil, nil); err == nil {
			t.Error("NewServerForMode(https, no servers) = nil error, want error")
		}
	})
}

// ── serving ──────────────────────────────────────────────────────────────

func TestServerForwardsPlainUpstreamUDPAndTCP(t *testing.T) {
	t.Parallel()
	upstreamAddr := startFakeUpstream(t)
	proxyAddr := startProxy(t, []Upstream{{proto: ProtoPlain, address: upstreamAddr}})

	query := testQuery()
	want := fakeResponse(query)

	if got := udpExchange(t, proxyAddr, query); !bytes.Equal(got, want) {
		t.Errorf("udp response = %q, want %q", got, want)
	}
	if got := tcpExchange(t, proxyAddr, query); !bytes.Equal(got, want) {
		t.Errorf("tcp response = %q, want %q", got, want)
	}
}

func TestServerFailsOverToHealthyUpstream(t *testing.T) {
	t.Parallel()
	upstreamAddr := startFakeUpstream(t)
	proxyAddr := startProxy(t, []Upstream{
		{proto: ProtoPlain, address: "127.0.0.1:1"}, // nothing listens here
		{proto: ProtoPlain, address: upstreamAddr},
	})

	query := testQuery()
	if got := udpExchange(t, proxyAddr, query); !bytes.Equal(got, fakeResponse(query)) {
		t.Errorf("udp response = %q, want the answer from the healthy upstream", got)
	}
}

func TestServerReturnsServfailWhenAllUpstreamsFail(t *testing.T) {
	t.Parallel()
	proxyAddr := startProxy(t, []Upstream{{proto: ProtoPlain, address: "127.0.0.1:1"}})

	query := testQuery()
	got := udpExchange(t, proxyAddr, query)
	if !bytes.Equal(got, servfail(query)) {
		t.Fatalf("response = %x, want SERVFAIL", got)
	}
	if got[3]&0x0f != 2 {
		t.Errorf("RCODE = %d, want 2 (SERVFAIL)", got[3]&0x0f)
	}
	if got[2]&0x80 == 0 {
		t.Error("QR bit not set on SERVFAIL response")
	}
}

func TestServerForwardsDoHUpstream(t *testing.T) {
	t.Parallel()

	var gotContentType string
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/dns-message")
		_, _ = w.Write(fakeResponse(body))
	}))
	defer ts.Close()

	u, err := ParseUpstream(ts.URL, ProtoHTTPS)
	if err != nil {
		t.Fatal(err)
	}
	u.httpClient = ts.Client() // trust the test certificate
	proxyAddr := startProxy(t, []Upstream{u})

	query := testQuery()
	if got := udpExchange(t, proxyAddr, query); !bytes.Equal(got, fakeResponse(query)) {
		t.Errorf("DoH response = %q, want the upstream answer", got)
	}
	if gotContentType != "application/dns-message" {
		t.Errorf("DoH Content-Type = %q, want application/dns-message", gotContentType)
	}
}

func TestServerForwardsDoTUpstream(t *testing.T) {
	t.Parallel()

	cert := selfSignedCert(t)
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(2 * time.Second))
				query, err := readFramed(c)
				if err != nil {
					return
				}
				_ = writeFramed(c, fakeResponse(query))
			}(conn)
		}
	}()

	proxyAddr := startProxy(t, []Upstream{{
		proto:     ProtoTLS,
		address:   ln.Addr().String(),
		tlsConfig: &tls.Config{InsecureSkipVerify: true},
	}})

	query := testQuery()
	if got := udpExchange(t, proxyAddr, query); !bytes.Equal(got, fakeResponse(query)) {
		t.Errorf("DoT response = %q, want the upstream answer", got)
	}
}

func TestServfailShape(t *testing.T) {
	t.Parallel()
	query := testQuery()
	resp := servfail(query)

	if len(resp) != len(query) {
		t.Fatalf("servfail length = %d, want %d", len(resp), len(query))
	}
	if binary.BigEndian.Uint16(resp[0:2]) != binary.BigEndian.Uint16(query[0:2]) {
		t.Error("servfail did not echo the query ID")
	}
	if resp[2]&0x80 == 0 {
		t.Error("QR bit not set")
	}
	if resp[3]&0x0f != 0x02 {
		t.Errorf("RCODE = %d, want 2", resp[3]&0x0f)
	}
	if resp[3]&0x80 == 0 {
		t.Error("RA bit not set")
	}
	for _, off := range []int{6, 8, 10} {
		if binary.BigEndian.Uint16(resp[off:off+2]) != 0 {
			t.Errorf("record count at offset %d not zeroed", off)
		}
	}
}

func TestServfailRejectsShortQuery(t *testing.T) {
	t.Parallel()
	if got := servfail([]byte{0x00, 0x01}); got != nil {
		t.Errorf("servfail(short) = %v, want nil", got)
	}
}
