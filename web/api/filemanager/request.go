package filemanager

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/monitor-monitor/monitor/database/clients"
	v2 "github.com/monitor-monitor/monitor/protocol/v2"
	"github.com/monitor-monitor/monitor/utils"
	agentRuntime "github.com/monitor-monitor/monitor/web/agent"
	"github.com/monitor-monitor/monitor/web/api"
)

func RequestFileManager(c *gin.Context) {
	uuid := c.Param("uuid")
	if _, err := clients.GetClientByUUID(uuid); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Client not found"})
		return
	}
	if !api.IsWebSocketUpgrade(c) {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Require WebSocket upgrade"})
		return
	}
	browser, err := api.UpgradeWebSocket(c)
	if err != nil {
		return
	}
	browser.SetReadLimit(maxFileMessageSize)

	userValue, _ := c.Get("uuid")
	userUUID, _ := userValue.(string)
	id := utils.GenerateRandomString(32)
	session := &fileSession{
		UUID: uuid, UserUUID: userUUID, Browser: browser, RequesterIP: c.ClientIP(),
	}
	fileSessionsMu.Lock()
	fileSessions[id] = session
	fileSessionsMu.Unlock()

	browser.SetCloseHandler(func(code int, text string) error {
		log.Println("File manager connection closed:", code, text)
		closeFileSession(id, session)
		return nil
	})

	agentConn := agentRuntime.GetConnectedClients()[uuid]
	if agentConn == nil {
		_ = browser.WriteJSON(gin.H{"type": "system", "ok": false, "error": "节点离线"})
		closeFileSession(id, session)
		return
	}

	if !agentRuntime.IsV2Client(uuid) || !agentRuntime.DispatchV2Event(uuid, v2.MethodAgentFile, v2.FileRequestParams{RequestID: id}) {
		_ = browser.WriteJSON(gin.H{"type": "system", "ok": false, "error": "节点不支持 v2 通道"})
		closeFileSession(id, session)
		return
	}
	_ = browser.WriteJSON(gin.H{"type": "system", "ok": true, "status": "waiting"})
	waitForAgent(browser, session, id)
}

func waitForAgent(browser *websocket.Conn, session *fileSession, id string) {
	time.AfterFunc(30*time.Second, func() {
		fileSessionsMu.Lock()
		current, ok := fileSessions[id]
		if ok && current == session && session.Agent == nil {
			delete(fileSessions, id)
			fileSessionsMu.Unlock()
			_ = browser.WriteJSON(gin.H{"type": "system", "ok": false, "error": "等待 Agent 超时"})
			_ = browser.Close()
			return
		}
		fileSessionsMu.Unlock()
	})
}

func closeFileSession(id string, session *fileSession) {
	fileSessionsMu.Lock()
	current, ok := fileSessions[id]
	if ok && current == session {
		delete(fileSessions, id)
	}
	agent := session.Agent
	browser := session.Browser
	fileSessionsMu.Unlock()
	if agent != nil {
		_ = agent.Close()
	}
	if browser != nil {
		_ = browser.Close()
	}
}
