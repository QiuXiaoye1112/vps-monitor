package terminal

import (
	"sync"
	"time"

	"github.com/monitor-monitor/monitor/web/connection"
)

type TerminalSession struct {
	UUID        string
	UserUUID    string
	Browser     *connection.SafeConn
	Agent       *connection.SafeConn
	RequesterIp string
	closeOnce   sync.Once
	waitTimer   *time.Timer
}

var TerminalSessionsMutex = &sync.Mutex{}
var TerminalSessions = make(map[string]*TerminalSession)

func getTerminalSession(id string) (*TerminalSession, bool) {
	TerminalSessionsMutex.Lock()
	defer TerminalSessionsMutex.Unlock()
	session, exists := TerminalSessions[id]
	return session, exists
}

func closeTerminalSession(id string, session *TerminalSession) {
	if session == nil {
		return
	}

	session.closeOnce.Do(func() {
		TerminalSessionsMutex.Lock()
		if current, exists := TerminalSessions[id]; exists && current == session {
			delete(TerminalSessions, id)
		}
		agent := session.Agent
		browser := session.Browser
		waitTimer := session.waitTimer
		session.waitTimer = nil
		TerminalSessionsMutex.Unlock()

		if waitTimer != nil {
			waitTimer.Stop()
		}
		if agent != nil {
			_ = agent.Close()
		}
		if browser != nil {
			_ = browser.Close()
		}
	})
}
