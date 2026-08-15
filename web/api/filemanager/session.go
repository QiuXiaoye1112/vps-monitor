package filemanager

import (
	"sync"
	"time"

	"github.com/monitor-monitor/monitor/web/connection"
)

const maxFileMessageSize = 3 << 20

type fileSession struct {
	UUID        string
	UserUUID    string
	Browser     *connection.SafeConn
	Agent       *connection.SafeConn
	RequesterIP string
	closeOnce   sync.Once
	waitTimer   *time.Timer
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
