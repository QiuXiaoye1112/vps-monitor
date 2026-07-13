package filemanager

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/monitor-monitor/monitor/web/api"
)

func EstablishConnection(c *gin.Context) {
	id := c.Query("id")
	session, ok := getFileSession(id)
	if !ok || session == nil || session.Browser == nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "Session not found"})
		return
	}
	principal := api.GetPrincipal(c)
	if principal == nil || principal.ClientUUID == "" || principal.ClientUUID != session.UUID {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "Agent does not match session"})
		return
	}
	if !api.IsWebSocketUpgrade(c) {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Require WebSocket upgrade"})
		return
	}
	agent, err := api.UpgradeWebSocket(c)
	if err != nil {
		return
	}
	agent.SetReadLimit(maxFileMessageSize)

	fileSessionsMu.Lock()
	current, stillExists := fileSessions[id]
	if !stillExists || current != session || session.Browser == nil || session.Agent != nil {
		fileSessionsMu.Unlock()
		_ = agent.Close()
		return
	}
	session.Agent = agent
	fileSessionsMu.Unlock()

	_ = session.Browser.WriteJSON(gin.H{"type": "system", "ok": true, "status": "connected"})
	go forwardFileManager(id, session)
}
