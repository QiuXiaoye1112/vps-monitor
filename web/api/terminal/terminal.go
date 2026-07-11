package terminal

import (
	"sync"

	"github.com/gorilla/websocket"
)

type TerminalSession struct {
	UUID        string
	UserUUID    string
	Browser     *websocket.Conn
	Agent       *websocket.Conn
	RequesterIp string
}

var TerminalSessionsMutex = &sync.Mutex{}
var TerminalSessions = make(map[string]*TerminalSession)

func getTerminalSession(id string) (*TerminalSession, bool) {
	TerminalSessionsMutex.Lock()
	defer TerminalSessionsMutex.Unlock()
	session, exists := TerminalSessions[id]
	return session, exists
}

func deleteTerminalSession(id string) {
	TerminalSessionsMutex.Lock()
	delete(TerminalSessions, id)
	TerminalSessionsMutex.Unlock()
}
