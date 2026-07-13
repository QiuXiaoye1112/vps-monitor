package filemanager

import (
	"time"

	"github.com/monitor-monitor/monitor/database/auditlog"
)

func forwardFileManager(id string, session *fileSession) {
	auditlog.Log(session.RequesterIP, session.UserUUID, "established, file manager id:"+id, "file_manager")
	started := time.Now()
	errCh := make(chan error, 2)

	copyMessages := func(dst, src interface {
		ReadMessage() (messageType int, p []byte, err error)
		WriteMessage(messageType int, data []byte) error
	}) {
		for {
			messageType, data, err := src.ReadMessage()
			if err == nil {
				err = dst.WriteMessage(messageType, data)
			}
			if err != nil {
				errCh <- err
				return
			}
		}
	}

	go copyMessages(session.Agent, session.Browser)
	go copyMessages(session.Browser, session.Agent)
	<-errCh
	closeFileSession(id, session)
	auditlog.Log(session.RequesterIP, session.UserUUID, "disconnected, file manager id:"+id+", duration:"+time.Since(started).String(), "file_manager")
}
