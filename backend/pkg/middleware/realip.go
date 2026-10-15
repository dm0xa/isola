// Package middleware provides HTTP middleware shared by the isola server
// binaries.
package middleware

import (
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
)

// TrustedRealIP rewrites r.RemoteAddr from X-Forwarded-For / X-Real-IP, but
// only when the direct peer is a trusted reverse proxy. chi's RealIP
// middleware trusts those headers unconditionally, so any client can spoof
// its address and defeat the per-IP rate limiter (VULN-25). Trusted proxies
// are configured with TRUSTED_PROXY_CIDRS (comma-separated CIDRs or bare IPs);
// the default is loopback only, which is correct for a directly exposed
// binary. Deployments behind nginx must set TRUSTED_PROXY_CIDRS to the proxy
// network (e.g. 172.16.0.0/12 for Docker bridge networks).
func TrustedRealIP(next http.Handler) http.Handler {
	nets := trustedProxyNets()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peer := remoteIP(r.RemoteAddr)
		if peer != nil && ipInNets(peer, nets) {
			if ip := forwardedClientIP(r, nets); ip != "" {
				r.RemoteAddr = ip + ":0"
			}
		}
		next.ServeHTTP(w, r)
	})
}

var (
	trustedProxyOnce sync.Once
	trustedProxyList []*net.IPNet
)

func trustedProxyNets() []*net.IPNet {
	trustedProxyOnce.Do(func() {
		spec := strings.TrimSpace(os.Getenv("TRUSTED_PROXY_CIDRS"))
		if spec == "" {
			spec = "127.0.0.0/8,::1/128"
		}
		for _, part := range strings.Split(spec, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if !strings.Contains(part, "/") {
				if ip := net.ParseIP(part); ip != nil {
					if ip.To4() != nil {
						part += "/32"
					} else {
						part += "/128"
					}
				}
			}
			if _, n, err := net.ParseCIDR(part); err == nil {
				trustedProxyList = append(trustedProxyList, n)
			}
		}
	})
	return trustedProxyList
}

func remoteIP(remoteAddr string) net.IP {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	return net.ParseIP(strings.TrimSpace(host))
}

func ipInNets(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// forwardedClientIP walks X-Forwarded-For right-to-left and returns the first
// address that is not itself a trusted proxy, i.e. the closest real client.
// If every entry is trusted it falls back to X-Real-IP.
func forwardedClientIP(r *http.Request, nets []*net.IPNet) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			ip := net.ParseIP(strings.TrimSpace(parts[i]))
			if ip == nil {
				continue
			}
			if !ipInNets(ip, nets) {
				return ip.String()
			}
		}
	}
	if xr := strings.TrimSpace(r.Header.Get("X-Real-IP")); xr != "" {
		if ip := net.ParseIP(xr); ip != nil {
			return ip.String()
		}
	}
	return ""
}
