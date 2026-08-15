package terminal

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/monitor-monitor/monitor/web/connection"
)

func TestTerminalHeartbeatTiming(t *testing.T) {
	if terminalPingInterval != 25*time.Second {
		t.Fatalf("Ping interval = %s, want 25s", terminalPingInterval)
	}
	if terminalPongTimeout != 60*time.Second {
		t.Fatalf("Pong timeout = %s, want 60s", terminalPongTimeout)
	}
	if terminalWriteTimeout != 10*time.Second {
		t.Fatalf("write timeout = %s, want 10s", terminalWriteTimeout)
	}
}

func TestTerminalHeartbeatUsesControlPingAndRefreshesDeadline(t *testing.T) {
	monitorRaw, peer := newWebSocketPair(t)
	monitor := connection.NewSafeConn(monitorRaw)

	const pongTimeout = 250 * time.Millisecond
	if err := configureTerminalHeartbeat(monitor, pongTimeout); err != nil {
		t.Fatalf("configure heartbeat: %v", err)
	}

	pingReceived := make(chan struct{}, 1)
	peer.SetPingHandler(func(payload string) error {
		select {
		case pingReceived <- struct{}{}:
		default:
		}
		return peer.WriteControl(
			websocket.PongMessage,
			[]byte(payload),
			time.Now().Add(time.Second),
		)
	})

	type readResult struct {
		messageType int
		data        []byte
		err         error
	}
	monitorRead := make(chan readResult, 1)
	go func() {
		messageType, data, err := monitor.ReadMessage()
		monitorRead <- readResult{messageType: messageType, data: data, err: err}
	}()
	peerReadDone := make(chan struct{})
	go func() {
		defer close(peerReadDone)
		_, _, _ = peer.ReadMessage()
	}()

	ctx, cancel := context.WithCancel(context.Background())
	heartbeatDone := make(chan struct{})
	stopped := make(chan struct{}, 1)
	go func() {
		defer close(heartbeatDone)
		runTerminalHeartbeat(ctx, "test", monitor, 20*time.Millisecond, func() {
			select {
			case stopped <- struct{}{}:
			default:
			}
		})
	}()

	select {
	case <-pingReceived:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for WebSocket Ping control frame")
	}

	// This is longer than the initial deadline. Regular Pong frames must keep
	// the monitor reader alive until the data message arrives.
	time.Sleep(2 * pongTimeout)
	if err := peer.WriteMessage(websocket.TextMessage, []byte("terminal-data")); err != nil {
		t.Fatalf("write terminal data: %v", err)
	}

	select {
	case result := <-monitorRead:
		if result.err != nil {
			t.Fatalf("monitor read failed after Pong refresh: %v", result.err)
		}
		if result.messageType != websocket.TextMessage || string(result.data) != "terminal-data" {
			t.Fatalf("unexpected data frame: type=%d data=%q", result.messageType, result.data)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for terminal data")
	}

	select {
	case <-stopped:
		t.Fatal("heartbeat stopped while the peer was replying with Pong")
	default:
	}

	cancel()
	select {
	case <-heartbeatDone:
	case <-time.After(time.Second):
		t.Fatal("heartbeat goroutine did not stop after cancellation")
	}
	_ = peer.Close()
	select {
	case <-peerReadDone:
	case <-time.After(time.Second):
		t.Fatal("peer reader did not stop")
	}
}

func TestTerminalHeartbeatStopsWhenPingFails(t *testing.T) {
	monitorRaw, peer := newWebSocketPair(t)
	monitor := connection.NewSafeConn(monitorRaw)
	_ = peer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := make(chan struct{})
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		runTerminalHeartbeat(ctx, "test", monitor, 10*time.Millisecond, func() {
			close(stopped)
		})
	}()

	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("heartbeat did not stop after Ping write failure")
	}
	select {
	case <-heartbeatDone:
	case <-time.After(time.Second):
		t.Fatal("heartbeat goroutine did not exit after Ping write failure")
	}
}

func TestTerminalHeartbeatDoesNotConflictWithDataWrites(t *testing.T) {
	monitorRaw, peer := newWebSocketPair(t)
	monitor := connection.NewSafeConn(monitorRaw)

	const messageCount = 50
	peerReadDone := make(chan error, 1)
	go func() {
		for range messageCount {
			messageType, _, err := peer.ReadMessage()
			if err != nil {
				peerReadDone <- err
				return
			}
			if messageType != websocket.BinaryMessage {
				peerReadDone <- fmt.Errorf("unexpected WebSocket message type: %d", messageType)
				return
			}
		}
		peerReadDone <- nil
	}()

	ctx, cancel := context.WithCancel(context.Background())
	heartbeatDone := make(chan struct{})
	stopped := make(chan struct{}, 1)
	go func() {
		defer close(heartbeatDone)
		runTerminalHeartbeat(ctx, "test", monitor, time.Millisecond, func() {
			select {
			case stopped <- struct{}{}:
			default:
			}
		})
	}()

	payload := make([]byte, 4<<10)
	for range messageCount {
		if err := monitor.WriteMessage(websocket.BinaryMessage, payload); err != nil {
			t.Fatalf("write terminal data: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case err := <-peerReadDone:
		if err != nil {
			t.Fatalf("read terminal data: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out reading terminal data")
	}

	cancel()
	select {
	case <-heartbeatDone:
	case <-time.After(time.Second):
		t.Fatal("heartbeat goroutine did not stop")
	}
	select {
	case <-stopped:
		t.Fatal("heartbeat reported a Ping failure during data writes")
	default:
	}
}

func newWebSocketPair(t *testing.T) (*websocket.Conn, *websocket.Conn) {
	t.Helper()

	serverConn := make(chan *websocket.Conn, 1)
	releaseServer := make(chan struct{})
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serverConn <- conn
		<-releaseServer
	}))

	peer, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http"),
		nil,
	)
	if err != nil {
		close(releaseServer)
		server.Close()
		t.Fatalf("dial WebSocket: %v", err)
	}
	monitor := <-serverConn
	t.Cleanup(func() {
		_ = peer.Close()
		_ = monitor.Close()
		close(releaseServer)
		server.Close()
	})
	return monitor, peer
}
