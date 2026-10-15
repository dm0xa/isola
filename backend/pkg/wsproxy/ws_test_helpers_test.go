package wsproxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func newWebSocketPair(t *testing.T) (*websocket.Conn, *websocket.Conn, func()) {
	t.Helper()

	serverConnCh := make(chan *websocket.Conn, 1)
	serverErrCh := make(chan error, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			serverErrCh <- err
			return
		}
		serverConnCh <- conn
	}))

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	header := http.Header{"Origin": {srv.URL}}
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		srv.Close()
		t.Fatalf("websocket dial: %v", err)
	}

	var serverConn *websocket.Conn
	select {
	case serverConn = <-serverConnCh:
	case err := <-serverErrCh:
		clientConn.Close()
		srv.Close()
		t.Fatalf("websocket upgrade: %v", err)
	case <-time.After(3 * time.Second):
		clientConn.Close()
		srv.Close()
		t.Fatal("timed out waiting for upgraded server connection")
	}

	cleanup := func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
		srv.Close()
	}
	return serverConn, clientConn, cleanup
}
