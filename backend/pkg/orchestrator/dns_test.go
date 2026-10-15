package orchestrator

import (
	"reflect"
	"testing"
)

func TestDNSSessionArgs(t *testing.T) {
	t.Parallel()

	servers := []string{"1.1.1.1", "8.8.8.8"}

	tests := []struct {
		name     string
		mode     string
		proxyOn  bool
		proxyIP  string
		wantArgs []string
		wantFell bool
	}{
		{
			name:     "plain mode passes servers",
			mode:     "plain",
			wantArgs: []string{"--dns", "1.1.1.1", "--dns", "8.8.8.8"},
		},
		{
			name:     "tls mode points at the forwarder",
			mode:     "tls",
			proxyOn:  true,
			proxyIP:  "10.89.0.5",
			wantArgs: []string{"--dns", "10.89.0.5", "-e", "ISOLA_DNS_PROXY_IP=10.89.0.5"},
		},
		{
			name:     "https mode points at the forwarder",
			mode:     "https",
			proxyOn:  true,
			proxyIP:  "10.89.0.5",
			wantArgs: []string{"--dns", "10.89.0.5", "-e", "ISOLA_DNS_PROXY_IP=10.89.0.5"},
		},
		{
			name:     "tls mode without a forwarder falls back",
			mode:     "tls",
			proxyOn:  false,
			wantArgs: []string{"--dns", "1.1.1.1", "--dns", "8.8.8.8"},
			wantFell: true,
		},
		{
			name:     "tls mode without a known proxy IP falls back",
			mode:     "tls",
			proxyOn:  true,
			proxyIP:  "",
			wantArgs: []string{"--dns", "1.1.1.1", "--dns", "8.8.8.8"},
			wantFell: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			args, fell := dnsSessionArgs(tt.mode, tt.proxyOn, tt.proxyIP, servers)
			if !reflect.DeepEqual(args, tt.wantArgs) {
				t.Errorf("args = %v, want %v", args, tt.wantArgs)
			}
			if fell != tt.wantFell {
				t.Errorf("fellBack = %v, want %v", fell, tt.wantFell)
			}
		})
	}
}

func TestDNSArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		servers []string
		want    []string
	}{
		{
			name:    "empty means embedded resolver",
			servers: nil,
			want:    []string{},
		},
		{
			name:    "each server becomes a flag pair",
			servers: []string{"1.1.1.1", "8.8.8.8", "9.9.9.9"},
			want:    []string{"--dns", "1.1.1.1", "--dns", "8.8.8.8", "--dns", "9.9.9.9"},
		},
		{
			name:    "blank entries are dropped",
			servers: []string{"1.1.1.1", "", "  ", "8.8.8.8"},
			want:    []string{"--dns", "1.1.1.1", "--dns", "8.8.8.8"},
		},
		{
			name:    "whitespace is trimmed",
			servers: []string{" 9.9.9.9 "},
			want:    []string{"--dns", "9.9.9.9"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := dnsArgs(tt.servers); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("dnsArgs(%v) = %v, want %v", tt.servers, got, tt.want)
			}
		})
	}
}
