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
	"github.com/monitor-monitor/monitor/web/connection"
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
	browser := connection.NewSafeConn(conn)
	// 新建一个终端连接
	id := utils.GenerateRandomString(32)
	session := &TerminalSession{
		UserUUID:    userUUID,
		UUID:        uuid,
		Browser:     browser,
		Agent:       nil,
		RequesterIp: c.ClientIP(),
	}

	TerminalSessionsMutex.Lock()
	TerminalSessions[id] = session
	TerminalSessionsMutex.Unlock()
	conn.SetCloseHandler(func(code int, text string) error {
		log.Println("Terminal connection closed:", code, text)
		closeTerminalSession(id, session)
		return nil
	})

	agentConn := agent_runtime.GetConnectedClients()[uuid]
	if agentConn == nil {
		_ = browser.WriteMessage(1, []byte("Client offline!\n被控端离线!\n"))
		closeTerminalSession(id, session)
		return
	}

	if !agent_runtime.IsV2Client(uuid) || !agent_runtime.DispatchV2Event(uuid, v2.MethodAgentTerminal, v2.TerminalRequestParams{RequestID: id}) {
		_ = browser.WriteMessage(1, []byte("节点不支持 v2 通道\nClient does not support v2\n"))
		closeTerminalSession(id, session)
		return
	}
	_ = browser.WriteMessage(1, []byte("等待被控端连接 waiting for agent...\n"))
	// 如果没有连接上，则关闭连接
	waitForAgentConnection(session, id)
	//auditlog.Log(c.ClientIP(), userUUID, "request, terminal id:"+id+",client:"+session.UUID, "terminal")
}

func waitForAgentConnection(session *TerminalSession, id string) {
	timer := time.AfterFunc(30*time.Second, func() {
		TerminalSessionsMutex.Lock()
		current, exists := TerminalSessions[id]
		timedOut := exists && current == session && session.Agent == nil
		if timedOut {
			session.waitTimer = nil
		}
		TerminalSessionsMutex.Unlock()
		if timedOut {
			_ = session.Browser.WriteMessage(1, []byte("被控端连接超时 timeout\n"))
			closeTerminalSession(id, session)
		}
	})

	TerminalSessionsMutex.Lock()
	current, exists := TerminalSessions[id]
	if exists && current == session && session.Agent == nil {
		session.waitTimer = timer
		TerminalSessionsMutex.Unlock()
		return
	}
	TerminalSessionsMutex.Unlock()
	timer.Stop()
}
