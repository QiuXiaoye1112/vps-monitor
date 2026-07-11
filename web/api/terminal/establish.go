package terminal

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/monitor-monitor/monitor/web/api"
)

func EstablishConnection(c *gin.Context) {
	session_id := c.Query("id")
	session, exists := getTerminalSession(session_id)
	if !exists || session == nil || session.Browser == nil {
		c.JSON(404, gin.H{"status": "error", "error": "Session not found"})
		return
	}
	// Upgrade the connection to WebSocket
	if !api.IsWebSocketUpgrade(c) {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Require WebSocket upgrade"})
		return
	}
	conn, err := api.UpgradeWebSocket(c)
	if err != nil {
		TerminalSessionsMutex.Lock()
		if session.Browser != nil {
			session.Browser.Close()
		}
		delete(TerminalSessions, session_id)
		TerminalSessionsMutex.Unlock()
		return
	}
	TerminalSessionsMutex.Lock()
	current, stillExists := TerminalSessions[session_id]
	if !stillExists || current != session || session.Browser == nil {
		TerminalSessionsMutex.Unlock()
		conn.Close()
		return
	}
	session.Agent = conn
	TerminalSessionsMutex.Unlock()
	conn.SetCloseHandler(func(code int, text string) error {
		deleteTerminalSession(session_id)
		// 通知 Browser 关闭终端连接
		if session.Browser != nil {
			session.Browser.Close()
		}
		return nil
	})
	go ForwardTerminal(session_id)
}
