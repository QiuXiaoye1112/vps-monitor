package filemanager

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/monitor-monitor/monitor/web/connection"
)

func TestFileManagerHeartbeatTiming(t *testing.T) {
	if fileManagerPingInterval != 25*time.Second {
		t.Fatalf("Ping interval = %s, want 25s", fileManagerPingInterval)
	}
	if fileManagerPongTimeout != 60*time.Second {
		t.Fatalf("Pong timeout = %s, want 60s", fileManagerPongTimeout)
	}
	if fileManagerWriteTimeout != 10*time.Second {
		t.Fatalf("write timeout = %s, want 10s", fileManagerWriteTimeout)
	}
}

func TestFileManagerHeartbeatUsesControlPingAndRefreshesDeadline(t *testing.T) {
	monitorRaw, peer := newFileManagerWebSocketPair(t)
	monitor := connection.NewSafeConn(monitorRaw)

	const pongTimeout = 250 * time.Millisecond
	if err := configureFileManagerHeartbeat(monitor, pongTimeout); err != nil {
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
		runFileManagerHeartbeat(ctx, "test", monitor, 20*time.Millisecond, func() {
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
	// the monitor reader alive until the file data message arrives.
	time.Sleep(2 * pongTimeout)
	if err := peer.WriteMessage(websocket.TextMessage, []byte("file-data")); err != nil {
		t.Fatalf("write file data: %v", err)
	}

	select {
	case result := <-monitorRead:
		if result.err != nil {
			t.Fatalf("monitor read failed after Pong refresh: %v", result.err)
		}
		if result.messageType != websocket.TextMessage || string(result.data) != "file-data" {
			t.Fatalf("unexpected data frame: type=%d data=%q", result.messageType, result.data)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for file data")
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

func TestFileManagerHeartbeatStopsWhenPingFails(t *testing.T) {
	monitorRaw, peer := newFileManagerWebSocketPair(t)
	monitor := connection.NewSafeConn(monitorRaw)
	_ = peer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := make(chan struct{})
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		runFileManagerHeartbeat(ctx, "test", monitor, 10*time.Millisecond, func() {
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

func TestFileManagerHeartbeatReadDeadlineExpiresWithoutPong(t *testing.T) {
	monitorRaw, _ := newFileManagerWebSocketPair(t)
	monitor := connection.NewSafeConn(monitorRaw)
	if err := configureFileManagerHeartbeat(monitor, 50*time.Millisecond); err != nil {
		t.Fatalf("configure heartbeat: %v", err)
	}

	_, _, err := monitor.ReadMessage()
	if err == nil {
		t.Fatal("read remained active without Pong")
	}
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("read error = %v, want network timeout", err)
	}
}

func TestFileManagerHeartbeatDoesNotConflictWithDataWrites(t *testing.T) {
	monitorRaw, peer := newFileManagerWebSocketPair(t)
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
		runFileManagerHeartbeat(ctx, "test", monitor, time.Millisecond, func() {
			select {
			case stopped <- struct{}{}:
			default:
			}
		})
	}()

	payload := make([]byte, 4<<10)
	for range messageCount {
		if err := monitor.WriteMessage(websocket.BinaryMessage, payload); err != nil {
			t.Fatalf("write file data: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case err := <-peerReadDone:
		if err != nil {
			t.Fatalf("read file data: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out reading file data")
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

func TestCloseFileSessionStopsWaitTimerAndClosesConnections(t *testing.T) {
	browserRaw, browserPeer := newFileManagerWebSocketPair(t)
	agentRaw, agentPeer := newFileManagerWebSocketPair(t)
	timerFired := make(chan struct{}, 1)
	id := "close-file-session-test"
	session := &fileSession{
		Browser: connection.NewSafeConn(browserRaw),
		Agent:   connection.NewSafeConn(agentRaw),
		waitTimer: time.AfterFunc(100*time.Millisecond, func() {
			timerFired <- struct{}{}
		}),
	}
	fileSessionsMu.Lock()
	fileSessions[id] = session
	fileSessionsMu.Unlock()

	closeFileSession(id, session)
	closeFileSession(id, session)
	if _, exists := getFileSession(id); exists {
		t.Fatal("file manager session was not removed")
	}

	for side, peer := range map[string]*websocket.Conn{"browser": browserPeer, "agent": agentPeer} {
		_ = peer.SetReadDeadline(time.Now().Add(time.Second))
		if _, _, err := peer.ReadMessage(); err == nil {
			t.Fatalf("%s connection remained open", side)
		}
	}
	select {
	case <-timerFired:
		t.Fatal("wait timer fired after session close")
	case <-time.After(150 * time.Millisecond):
	}
}

func newFileManagerWebSocketPair(t *testing.T) (*websocket.Conn, *websocket.Conn) {
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
