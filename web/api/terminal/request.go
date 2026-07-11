package terminal

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/monitor-monitor/monitor/database/clients"
	v2 "github.com/monitor-monitor/monitor/protocol/v2"
	"github.com/monitor-monitor/monitor/utils"
	agent_runtime "github.com/monitor-monitor/monitor/web/agent"
	"github.com/monitor-monitor/monitor/web/api"
)

func RequestTerminal(c *gin.Context) {
	uuid := c.Param("uuid")
	userValue, _ := c.Get("uuid")
	userUUID, _ := userValue.(string)
	_, err := clients.GetClientByUUID(uuid)
	if err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "Client not found",
		})
		return
	}
	// 建立ws
	if !api.IsWebSocketUpgrade(c) {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Require WebSocket upgrade"})
		return
	}
	conn, err := api.UpgradeWebSocket(c)
	if err != nil {
		return
	}
	// 新建一个终端连接
	id := utils.GenerateRandomString(32)
	session := &TerminalSession{
		UserUUID:    userUUID,
		UUID:        uuid,
		Browser:     conn,
		Agent:       nil,
		RequesterIp: c.ClientIP(),
	}

	TerminalSessionsMutex.Lock()
	TerminalSessions[id] = session
	TerminalSessionsMutex.Unlock()
	conn.SetCloseHandler(func(code int, text string) error {
		log.Println("Terminal connection closed:", code, text)
		TerminalSessionsMutex.Lock()
		delete(TerminalSessions, id)
		agent := session.Agent
		TerminalSessionsMutex.Unlock()
		// 通知 Agent 关闭终端连接
		if agent != nil {
			agent.Close()
		}
		return nil
	})

	agentConn := agent_runtime.GetConnectedClients()[uuid]
	if agentConn == nil {
		conn.WriteMessage(1, []byte("Client offline!\n被控端离线!\n"))
		conn.Close()
		deleteTerminalSession(id)
		return
	}

	if agent_runtime.IsV2Client(uuid) {
		if agent_runtime.DispatchV2Event(uuid, v2.MethodAgentTerminal, v2.TerminalRequestParams{RequestID: id}) {
			conn.WriteMessage(1, []byte("等待被控端连接 waiting for agent...\n"))
			waitForAgentConnection(conn, session, id)
			return
		}
	}

	err = agentConn.WriteJSON(gin.H{
		"message":    "terminal",
		"request_id": id,
	})
	if err != nil {
		conn.Close()
		deleteTerminalSession(id)
		return
	}
	conn.WriteMessage(1, []byte("等待被控端连接 waiting for agent...\n"))
	// 如果没有连接上，则关闭连接
	waitForAgentConnection(conn, session, id)
	//auditlog.Log(c.ClientIP(), userUUID, "request, terminal id:"+id+",client:"+session.UUID, "terminal")
}

func waitForAgentConnection(conn interface{ Close() error }, session *TerminalSession, id string) {
	time.AfterFunc(30*time.Second, func() {
		TerminalSessionsMutex.Lock()
		if session.Agent == nil {
			if session.Browser != nil {
				session.Browser.WriteMessage(1, []byte("被控端连接超时 timeout\n"))
				session.Browser.Close()
			}
			conn.Close()
			delete(TerminalSessions, id)
		}
		TerminalSessionsMutex.Unlock()
	})
}
