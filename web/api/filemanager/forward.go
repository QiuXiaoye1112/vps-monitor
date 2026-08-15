package filemanager

import (
	"context"
	"errors"
	"log"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/monitor-monitor/monitor/database/auditlog"
	"github.com/monitor-monitor/monitor/web/connection"
)

const (
	fileManagerPingInterval = 25 * time.Second
	fileManagerPongTimeout  = 60 * time.Second
	fileManagerWriteTimeout = 10 * time.Second
)

func configureFileManagerHeartbeat(conn *connection.SafeConn, pongTimeout time.Duration) error {
	if err := conn.SetReadDeadline(time.Now().Add(pongTimeout)); err != nil {
		return err
	}
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongTimeout))
	})
	return nil
}

func runFileManagerHeartbeat(ctx context.Context, side string, conn *connection.SafeConn, interval time.Duration, stop func()) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(fileManagerWriteTimeout)); err != nil {
				log.Printf("file manager %s websocket ping failed: %v", side, err)
				stop()
				return
			}
		}
	}
}

func logFileManagerHeartbeatTimeout(side string, err error) {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		log.Printf("file manager %s websocket heartbeat timeout: %v", side, err)
	}
}

func forwardFileManager(id string, session *fileSession) {
	fileSessionsMu.Lock()
	current, exists := fileSessions[id]
	if !exists || current != session || session.Agent == nil || session.Browser == nil {
		fileSessionsMu.Unlock()
		return
	}
	agent := session.Agent
	browser := session.Browser
	fileSessionsMu.Unlock()

	if err := configureFileManagerHeartbeat(browser, fileManagerPongTimeout); err != nil {
		log.Printf("file manager browser websocket heartbeat setup failed: %v", err)
		closeFileSession(id, session)
		return
	}
	if err := configureFileManagerHeartbeat(agent, fileManagerPongTimeout); err != nil {
		log.Printf("file manager agent websocket heartbeat setup failed: %v", err)
		closeFileSession(id, session)
		return
	}

	auditlog.Log(session.RequesterIP, session.UserUUID, "established, file manager id:"+id, "file_manager")
	started := time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			cancel()
			closeFileSession(id, session)
		})
	}

	var workers sync.WaitGroup
	copyMessages := func(side string, dst, src *connection.SafeConn) {
		defer workers.Done()
		for {
			messageType, data, err := src.ReadMessage()
			if err != nil {
				logFileManagerHeartbeatTimeout(side, err)
				stop()
				return
			}
			if err := dst.WriteMessage(messageType, data); err != nil {
				stop()
				return
			}
		}
	}

	workers.Add(4)
	go func() {
		defer workers.Done()
		runFileManagerHeartbeat(ctx, "browser", browser, fileManagerPingInterval, stop)
	}()
	go func() {
		defer workers.Done()
		runFileManagerHeartbeat(ctx, "agent", agent, fileManagerPingInterval, stop)
	}()
	go copyMessages("browser", agent, browser)
	go copyMessages("agent", browser, agent)

	<-ctx.Done()
	workers.Wait()
	auditlog.Log(session.RequesterIP, session.UserUUID, "disconnected, file manager id:"+id+", duration:"+time.Since(started).String(), "file_manager")
}
