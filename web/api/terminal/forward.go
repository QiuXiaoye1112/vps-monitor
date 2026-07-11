package terminal

import (
	"time"

	"github.com/gorilla/websocket"
	"github.com/monitor-monitor/monitor/database/auditlog"
)

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
	auditlog.Log(session.RequesterIp, session.UserUUID, "established, terminal id:"+id, "terminal")
	established_time := time.Now()
	errChan := make(chan error, 1)

	go func() {
		for {
			messageType, data, err := browser.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}

			if messageType == websocket.TextMessage {
				if len(data) > 0 && data[0] == '{' {
					err = agent.WriteMessage(websocket.TextMessage, data)
				} else {
					err = agent.WriteMessage(websocket.BinaryMessage, data)
				}
			} else {
				// 二进制消息，原样传递
				err = agent.WriteMessage(websocket.BinaryMessage, data)
			}

			if err != nil {
				errChan <- err
				return
			}
		}
	}()

	go func() {
		for {
			_, data, err := agent.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}
			err = browser.WriteMessage(websocket.BinaryMessage, data)
			if err != nil {
				errChan <- err
				return
			}
		}
	}()

	// 等待错误或主动关闭
	<-errChan
	// 关闭连接
	agent.Close()
	browser.Close()
	disconnect_time := time.Now()
	auditlog.Log(session.RequesterIp, session.UserUUID, "disconnected, terminal id:"+id+", duration:"+disconnect_time.Sub(established_time).String(), "terminal")
	deleteTerminalSession(id)
}
