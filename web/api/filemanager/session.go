package filemanager

import (
	"sync"

	"github.com/gorilla/websocket"
)

const maxFileMessageSize = 3 << 20

type fileSession struct {
	UUID        string
	UserUUID    string
	Browser     *websocket.Conn
	Agent       *websocket.Conn
	RequesterIP string
}

var (
	fileSessionsMu sync.Mutex
	fileSessions   = make(map[string]*fileSession)
)

func getFileSession(id string) (*fileSession, bool) {
	fileSessionsMu.Lock()
	defer fileSessionsMu.Unlock()
	session, ok := fileSessions[id]
	return session, ok
}

func deleteFileSession(id string) {
	fileSessionsMu.Lock()
	delete(fileSessions, id)
	fileSessionsMu.Unlock()
}
