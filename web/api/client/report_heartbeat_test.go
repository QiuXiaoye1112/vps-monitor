package client

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/monitor-monitor/monitor/web/connection"
)

func TestClientHeartbeatTiming(t *testing.T) {
	if readWait != 65*time.Second {
		t.Fatalf("read wait = %s, want 65s", readWait)
	}
	if pongWriteWait != 10*time.Second {
		t.Fatalf("Pong write wait = %s, want 10s", pongWriteWait)
	}
}

func TestClientHeartbeatEchoesPingPayload(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawConn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn := connection.NewSafeConn(rawConn)
		defer conn.Close()
		configureClientHeartbeat(conn)
		_, _, _ = conn.ReadMessage()
	}))
	defer server.Close()

	conn, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http"),
		nil,
	)
	if err != nil {
		t.Fatalf("dial WebSocket: %v", err)
	}
	defer conn.Close()

	pongPayloads := make(chan string, 1)
	conn.SetPongHandler(func(payload string) error {
		pongPayloads <- payload
		return nil
	})
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		_, _, _ = conn.ReadMessage()
	}()

	const payload = "server-heartbeat-test"
	if err := conn.WriteControl(
		websocket.PingMessage,
		[]byte(payload),
		time.Now().Add(time.Second),
	); err != nil {
		t.Fatalf("write Ping: %v", err)
	}
	select {
	case pong := <-pongPayloads:
		if pong != payload {
			t.Fatalf("Pong payload = %q, want %q", pong, payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Pong")
	}

	_ = conn.Close()
	select {
	case <-readDone:
	case <-time.After(time.Second):
		t.Fatal("client reader did not stop")
	}
}
