package modulehost

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// rpcClient speaks contract v1 to one module over its UDS. Connections are
// not pooled (DisableKeepAlives) so a module restart is picked up on the
// next call without stale-connection errors; module traffic is low-QPS by
// design (video/streaming never crosses the boundary).
type rpcClient struct {
	socket string
	hc     *http.Client
	base   string // "http://isola-module"
}

const rpcTimeout = 5 * time.Second

func newRPCClient(socket string) *rpcClient {
	return &rpcClient{
		socket: socket,
		base:   "http://isola-module",
		hc: &http.Client{
			Timeout: rpcTimeout,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "unix", socket)
				},
				DisableKeepAlives: true,
			},
		},
	}
}

// apiError is a structured non-2xx module response.
type apiError struct {
	StatusCode int
	Reason     string
}

func (e *apiError) Error() string {
	if e.Reason != "" {
		return e.Reason
	}
	return fmt.Sprintf("module returned HTTP %d", e.StatusCode)
}

// post sends in as JSON and decodes a 2xx body into out (if non-nil).
func (c *rpcClient) post(ctx context.Context, path string, in, out interface{}) error {
	body, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("modulehost: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

// get fetches a JSON document into out.
func (c *rpcClient) get(ctx context.Context, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func (c *rpcClient) do(req *http.Request, out interface{}) error {
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("modulehost: module unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var eb struct {
			Reason string `json:"reason"`
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		json.Unmarshal(raw, &eb)
		return &apiError{StatusCode: resp.StatusCode, Reason: eb.Reason}
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out); err != nil {
		return fmt.Errorf("modulehost: decode module response: %w", err)
	}
	return nil
}

// transport exposes the raw HTTP transport so the WS pipe can hijack a
// connection with the original upgrade headers.
func (c *rpcClient) transport() *http.Transport {
	return c.hc.Transport.(*http.Transport)
}
