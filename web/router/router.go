package router

import (
	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/web/api"
	"github.com/komari-monitor/komari/web/api/admin"
	"github.com/komari-monitor/komari/web/api/client"
	public_api "github.com/komari-monitor/komari/web/api/public"
	"github.com/komari-monitor/komari/web/api/terminal"
	"github.com/komari-monitor/komari/web/public"
	jsonRpc "github.com/komari-monitor/komari/web/rpc/jsonrpc"
)

// Register binds all HTTP, WebSocket, JSON-RPC and static frontend routes.
//
// 设计：JSON 类接口统一经声明式路由桥 jsonRpc.Bind 绑定到对应 RPC2 方法，
// 不再有 per-resource gin handler 层。仅二进制/流/重定向/特殊鉴权类接口保留为 REST handler。
func Register(r *gin.Engine) {
	r.Any("/ping", func(c *gin.Context) {
		c.String(200, "pong")
	})

	registerPublicRoutes(r)
	registerAgentRoutes(r)
	registerAdminRoutes(r)

	public.Static(r.Group("/"), func(handlers ...gin.HandlerFunc) {
		r.NoRoute(handlers...)
	})
}

func removedAPIRoute(c *gin.Context) {
	c.JSON(404, gin.H{"status": "error", "message": "feature removed"})
}

// registerPublicRoutes 公开路由。JSON 读接口经 Bind 绑定到 public: 命名空间方法。
func registerPublicRoutes(r *gin.Engine) {
	for _, path := range []string{
		"/api/oauth",
		"/api/oauth_callback",
		"/api/admin/theme/upload",
		"/api/admin/theme/list",
		"/api/admin/theme/delete",
		"/api/admin/theme/set",
		"/api/admin/theme/update",
		"/api/admin/theme/import",
		"/api/admin/settings/oidc",
		"/api/admin/settings/cloudflared",
		"/api/admin/settings/cloudflared/start",
		"/api/admin/settings/cloudflared/stop",
		"/api/admin/settings/cloudflared/remove-token",
		"/api/admin/update/mmdb",
		"/api/admin/update/favicon",
	} {
		r.Any(path, removedAPIRoute)
	}
	r.Any("/api/admin/2fa/*path", removedAPIRoute)
	r.Any("/api/admin/oauth2/*path", removedAPIRoute)

	// 非 JSON / 特殊流程，保留 REST handler。
	r.POST("/api/login", public_api.Login)
	r.GET("/api/logout", public_api.Logout)
	r.GET("/api/mjpeg_live", public_api.MjpegLiveHandler)
	r.POST("/api/avatar", api.RequireRole(api.RoleAdmin), admin.UploadAvatar)
	// /api/clients 是 WebSocket 端点（客户端发 "get"/"get <uuid>" 拉取在线列表与最新上报），
	// 非 JSON-RPC，保留为 WS handler。
	r.GET("/api/clients", api.GetClients)

	// JSON 接口 -> RPC2。
	r.GET("/api/me", jsonRpc.Bind("public:getMe", jsonRpc.WithRaw()))
	r.GET("/api/nodes", jsonRpc.Bind("public:getNodesInformation"))
	r.GET("/api/public", jsonRpc.Bind("public:getPublicSettings"))
	r.GET("/api/version", jsonRpc.Bind("public:getVersion"))
	r.GET("/api/recent/:uuid", jsonRpc.Bind("public:getClientRecentRecords", jsonRpc.WithPath("uuid")))
	r.GET("/api/records/load", jsonRpc.Bind("public:getRecordsByUUID", jsonRpc.WithQuery("uuid", "load_type", "hours")))
	r.GET("/api/records/ping", jsonRpc.Bind("public:getPingRecords", jsonRpc.WithQuery("uuid", "task_id", "hours")))
	r.GET("/api/task/ping", jsonRpc.Bind("public:getPublicPingTasks"))

	// JSON-RPC 直连入口。
	r.GET("/api/rpc2", jsonRpc.OnRpcRequest)
	r.POST("/api/rpc2", jsonRpc.OnRpcRequest)
	r.GET("/rpc2", jsonRpc.OnRpcRequest)
	r.POST("/rpc2", jsonRpc.OnRpcRequest)
}

// registerAgentRoutes agent（客户端）上报与拉取路由。
func registerAgentRoutes(r *gin.Engine) {
	// AutoDiscovery 注册使用独立的 Authorization key 鉴权，保留 REST handler。
	r.POST("/api/clients/register", client.RegisterClient)

	tokenAuthorized := r.Group("/api/clients", api.RequireRole(api.RoleAdmin, api.RoleClient))
	{
		// 上报类（WS / 原始流 / 兼容协议）保留 REST handler。
		tokenAuthorized.GET("/report", client.WebSocketReport)
		tokenAuthorized.POST("/uploadBasicInfo", client.UploadBasicInfo)
		tokenAuthorized.POST("/report", client.UploadReport)
		tokenAuthorized.GET("/v2/rpc", client.WebSocketV2RPC)
		tokenAuthorized.POST("/v2/rpc", client.UploadV2RPC)
		tokenAuthorized.GET("/terminal", terminal.EstablishConnection)

		// JSON 接口 -> RPC2 (client: 命名空间)。
		tokenAuthorized.POST("/task/result", jsonRpc.Bind("client:taskResult", jsonRpc.WithRaw()))
		tokenAuthorized.GET("/ping/tasks", jsonRpc.Bind("client:getPingTasks", jsonRpc.WithRaw()))
		tokenAuthorized.POST("/ping/result", jsonRpc.Bind("client:uploadPingResult", jsonRpc.WithRaw()))
	}
}

// registerAdminRoutes 管理员路由。除二进制/流类外全部经 Bind 绑定到 admin: 命名空间方法。
func registerAdminRoutes(r *gin.Engine) {
	g := r.Group("/api/admin", api.RequireRole(api.RoleAdmin))

	// --- 二进制/流/重定向类，保留 REST handler ---
	g.GET("/download/backup", admin.DownloadBackup)
	g.POST("/upload/backup", admin.UploadBackup)
	g.GET("/test/geoip", jsonRpc.Bind("admin:testGeoip", jsonRpc.WithQuery("ip")))
	g.POST("/update/user", admin.UpdateUser)

	// 仅保留主题自定义设置。主题上传/切换已合并进默认 VPS 主题，不再暴露。
	theme := g.Group("/theme")
	{
		theme.GET("/settings", admin.GetThemeSettings)
		theme.POST("/settings", admin.UpdateThemeSettings)
	}

	// --- 以下全部 JSON -> RPC2 ---

	// tasks（远程执行）
	task := g.Group("/task")
	{
		task.GET("/all", jsonRpc.Bind("admin:getTasks"))
		task.POST("/exec", api.RequireSensitive2FA(), jsonRpc.Bind("admin:exec"))
		task.GET("/:task_id", jsonRpc.Bind("admin:getTaskById", jsonRpc.WithPath("task_id")))
		task.GET("/:task_id/result", jsonRpc.Bind("admin:getTaskResultsByTaskId", jsonRpc.WithPath("task_id")))
		task.GET("/:task_id/result/:uuid", jsonRpc.Bind("admin:getSpecificTaskResult", jsonRpc.WithPath("task_id", "uuid")))
		task.GET("/client/:uuid", jsonRpc.Bind("admin:getTasksByClientId", jsonRpc.WithPath("uuid")))
	}

	// settings
	settings := g.Group("/settings")
	{
		settings.GET("/", jsonRpc.Bind("admin:getSettings"))
		settings.POST("/", jsonRpc.Bind("admin:editSettings"))
		settings.GET("/xtermjs", jsonRpc.Bind("admin:getXtermjsSettings"))
		settings.POST("/xtermjs", jsonRpc.Bind("admin:setXtermjsSettings", jsonRpc.WithMessage("settings saved")))
	}

	// database 运维（压缩/大小）
	databaseGroup := g.Group("/database")
	{
		databaseGroup.GET("/size", jsonRpc.Bind("admin:getDatabaseSize"))
		databaseGroup.POST("/vacuum", jsonRpc.Bind("admin:vacuumDatabase", jsonRpc.WithMessage("database vacuumed")))
	}

	// clients
	clientGroup := g.Group("/client")
	{
		clientGroup.POST("/add", jsonRpc.Bind("admin:addClient", jsonRpc.WithFlat()))
		clientGroup.GET("/list", jsonRpc.Bind("admin:listClients", jsonRpc.WithRaw()))
		clientGroup.GET("/:uuid", jsonRpc.Bind("admin:getClient", jsonRpc.WithPath("uuid"), jsonRpc.WithRaw()))
		clientGroup.POST("/:uuid/edit", jsonRpc.Bind("admin:editClient", jsonRpc.WithPath("uuid")))
		clientGroup.POST("/:uuid/remove", jsonRpc.Bind("admin:removeClient", jsonRpc.WithPath("uuid")))
		clientGroup.GET("/:uuid/token", jsonRpc.Bind("admin:getClientToken", jsonRpc.WithPath("uuid"), jsonRpc.WithFlat()))
		clientGroup.POST("/order", jsonRpc.Bind("admin:orderClients"))
		clientGroup.GET("/:uuid/terminal", api.RequireSensitive2FA(), terminal.RequestTerminal)
	}

	// records
	record := g.Group("/record")
	{
		record.POST("/clear", jsonRpc.Bind("admin:clearRecords"))
		record.POST("/clear/all", jsonRpc.Bind("admin:clearAllRecords"))
	}

	// sessions
	session := g.Group("/session")
	{
		session.GET("/get", jsonRpc.Bind("admin:getSessions", jsonRpc.WithFlat()))
		session.POST("/remove", jsonRpc.Bind("admin:deleteSession"))
		session.POST("/remove/all", jsonRpc.Bind("admin:deleteAllSessions"))
	}

	g.GET("/logs", jsonRpc.Bind("admin:getLogs", jsonRpc.WithQuery("limit", "page")))

	// clipboard
	clipboardGroup := g.Group("/clipboard")
	{
		clipboardGroup.GET("/:id", jsonRpc.Bind("admin:getClipboard", jsonRpc.WithPath("id")))
		clipboardGroup.GET("", jsonRpc.Bind("admin:listClipboard"))
		clipboardGroup.POST("", jsonRpc.Bind("admin:createClipboard"))
		clipboardGroup.POST("/:id", jsonRpc.Bind("admin:updateClipboard", jsonRpc.WithPath("id")))
		clipboardGroup.POST("/remove", jsonRpc.Bind("admin:batchDeleteClipboard"))
		clipboardGroup.POST("/:id/remove", jsonRpc.Bind("admin:deleteClipboard", jsonRpc.WithPath("id")))
	}

	// ping tasks
	pingTask := g.Group("/ping")
	{
		pingTask.GET("/", jsonRpc.Bind("admin:getAllPingTasks"))
		pingTask.POST("/add", jsonRpc.Bind("admin:addPingTask"))
		pingTask.POST("/delete", jsonRpc.Bind("admin:deletePingTask"))
		pingTask.POST("/edit", jsonRpc.Bind("admin:editPingTask"))
		pingTask.POST("/order", jsonRpc.Bind("admin:orderPingTask"))
	}
}
