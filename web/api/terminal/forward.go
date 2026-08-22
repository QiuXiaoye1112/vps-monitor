package terminal

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
	terminalPingInterval = 25 * time.Second
	terminalPongTimeout  = 60 * time.Second
	terminalWriteTimeout = 10 * time.Second
)

func configureTerminalHeartbeat(conn *connection.SafeConn, pongTimeout time.Duration) error {
	if err := conn.SetReadDeadline(time.Now().Add(pongTimeout)); err != nil {
		return err
	}
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongTimeout))
	})
	return nil
}

func runTerminalHeartbeat(ctx context.Context, side string, conn *connection.SafeConn, interval time.Duration, stop func()) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(terminalWriteTimeout)); err != nil {
				log.Printf("terminal %s websocket ping failed: %v", side, err)
				stop()
				return
			}
		}
	}
}

func logTerminalHeartbeatTimeout(side string, err error) {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		log.Printf("terminal %s websocket heartbeat timeout: %v", side, err)
	}
}

func ForwardTerminal(id string) {
	TerminalSessionsMutex.Lock()
	session, exists := TerminalSessions[id]
	if !exists || session == nil || session.Agent == nil || session.Browser == nil {
		TerminalSessionsMutex.Unlock()
		return
	}
	agent := session.Agent
	browser := session.Browser
	TerminalSessionsMutex.Unlock()

	if err := configureTerminalHeartbeat(browser, terminalPongTimeout); err != nil {
		log.Printf("terminal browser websocket heartbeat setup failed: %v", err)
		closeTerminalSession(id, session)
		return
	}
	if err := configureTerminalHeartbeat(agent, terminalPongTimeout); err != nil {
		log.Printf("terminal agent websocket heartbeat setup failed: %v", err)
		closeTerminalSession(id, session)
		return
	}

	auditlog.Log(session.RequesterIp, session.UserUUID, "established, terminal id:"+id, "terminal")
	establishedAt := time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			cancel()
			closeTerminalSession(id, session)
		})
	}

	var workers sync.WaitGroup
	workers.Add(4)
	go func() {
		defer workers.Done()
		runTerminalHeartbeat(ctx, "browser", browser, terminalPingInterval, stop)
	}()
	go func() {
		defer workers.Done()
		runTerminalHeartbeat(ctx, "agent", agent, terminalPingInterval, stop)
	}()
	go func() {
		defer workers.Done()
		for {
			messageType, data, err := browser.ReadMessage()
			if err != nil {
				logTerminalHeartbeatTimeout("browser", err)
				stop()
				return
			}

			if messageType == websocket.TextMessage {
				if len(data) > 0 && data[0] == '{' {
					err = agent.WriteMessage(websocket.TextMessage, data)
				} else {
					err = agent.WriteMessage(websocket.BinaryMessage, data)
				}
			} else {
				err = agent.WriteMessage(websocket.BinaryMessage, data)
			}
			if err != nil {
				stop()
				return
			}
		}
	}()
	go func() {
		defer workers.Done()
		for {
			messageType, data, err := agent.ReadMessage()
			if err != nil {
				logTerminalHeartbeatTimeout("agent", err)
				stop()
				return
			}
			if err := browser.WriteMessage(messageType, data); err != nil {
				stop()
				return
			}
		}
	}()

	<-ctx.Done()
	workers.Wait()
	auditlog.Log(session.RequesterIp, session.UserUUID, "disconnected, terminal id:"+id+", duration:"+time.Since(establishedAt).String(), "terminal")
}
