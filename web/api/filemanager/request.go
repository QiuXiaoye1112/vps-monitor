package filemanager

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/monitor-monitor/monitor/database/clients"
	v2 "github.com/monitor-monitor/monitor/protocol/v2"
	"github.com/monitor-monitor/monitor/utils"
	agentRuntime "github.com/monitor-monitor/monitor/web/agent"
	"github.com/monitor-monitor/monitor/web/api"
	"github.com/monitor-monitor/monitor/web/connection"
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
	safeBrowser := connection.NewSafeConn(browser)

	userValue, _ := c.Get("uuid")
	userUUID, _ := userValue.(string)
	id := utils.GenerateRandomString(32)
	session := &fileSession{
		UUID: uuid, UserUUID: userUUID, Browser: safeBrowser, RequesterIP: c.ClientIP(),
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
		_ = safeBrowser.WriteJSON(gin.H{"type": "system", "ok": false, "error": "节点离线"})
		closeFileSession(id, session)
		return
	}

	if !agentRuntime.IsV2Client(uuid) || !agentRuntime.DispatchV2Event(uuid, v2.MethodAgentFile, v2.FileRequestParams{RequestID: id}) {
		_ = safeBrowser.WriteJSON(gin.H{"type": "system", "ok": false, "error": "节点不支持 v2 通道"})
		closeFileSession(id, session)
		return
	}
	_ = safeBrowser.WriteJSON(gin.H{"type": "system", "ok": true, "status": "waiting"})
	waitForAgent(session, id)
}

func waitForAgent(session *fileSession, id string) {
	timer := time.AfterFunc(30*time.Second, func() {
		fileSessionsMu.Lock()
		current, ok := fileSessions[id]
		timedOut := ok && current == session && session.Agent == nil
		if timedOut {
			session.waitTimer = nil
		}
		fileSessionsMu.Unlock()
		if timedOut {
			_ = session.Browser.WriteJSON(gin.H{"type": "system", "ok": false, "error": "等待 Agent 超时"})
			closeFileSession(id, session)
		}
	})

	fileSessionsMu.Lock()
	current, ok := fileSessions[id]
	if ok && current == session && session.Agent == nil {
		session.waitTimer = timer
		fileSessionsMu.Unlock()
		return
	}
	fileSessionsMu.Unlock()
	timer.Stop()
}

func closeFileSession(id string, session *fileSession) {
	if session == nil {
		return
	}

	session.closeOnce.Do(func() {
		fileSessionsMu.Lock()
		if current, ok := fileSessions[id]; ok && current == session {
			delete(fileSessions, id)
		}
		agent := session.Agent
		browser := session.Browser
		waitTimer := session.waitTimer
		session.waitTimer = nil
		fileSessionsMu.Unlock()

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
