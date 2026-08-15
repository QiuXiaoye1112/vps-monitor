package terminal

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/monitor-monitor/monitor/web/api"
	"github.com/monitor-monitor/monitor/web/connection"
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
		closeTerminalSession(session_id, session)
		return
	}
	agent := connection.NewSafeConn(conn)
	TerminalSessionsMutex.Lock()
	current, stillExists := TerminalSessions[session_id]
	if !stillExists || current != session || session.Browser == nil {
		TerminalSessionsMutex.Unlock()
		_ = agent.Close()
		return
	}
	session.Agent = agent
	waitTimer := session.waitTimer
	session.waitTimer = nil
	TerminalSessionsMutex.Unlock()
	if waitTimer != nil {
		waitTimer.Stop()
	}
	conn.SetCloseHandler(func(code int, text string) error {
		closeTerminalSession(session_id, session)
		return nil
	})
	go ForwardTerminal(session_id)
}
