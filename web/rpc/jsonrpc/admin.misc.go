package jsonrpc

import (
	"context"
	"strconv"

	"github.com/monitor-monitor/monitor/database/accounts"
	"github.com/monitor-monitor/monitor/database/auditlog"
	"github.com/monitor-monitor/monitor/database/dbcore"
	"github.com/monitor-monitor/monitor/database/models"
	"github.com/monitor-monitor/monitor/database/records"
	"github.com/monitor-monitor/monitor/database/tasks"
	"github.com/monitor-monitor/monitor/pkg/config"
	"github.com/monitor-monitor/monitor/pkg/rpc"
)

// admin.misc.go
// 杂项 admin RPC2 方法：会话管理、设置、客户端排序。

func parseUintKey(s string) (uint, error) {
	v, err := strconv.ParseUint(s, 10, 64)
	return uint(v), err
}

func init() {
	RegisterWithGroupAndMeta("getSessions", rpc.RoleAdmin, adminGetSessions, &rpc.MethodMeta{
		Name:    "admin:getSessions",
		Summary: "List all login sessions",
		Returns: "{ current: string, data: Session[] }",
	})
	RegisterWithGroupAndMeta("deleteSession", rpc.RoleAdmin, adminDeleteSession, &rpc.MethodMeta{
		Name:    "admin:deleteSession",
		Summary: "Delete a session by token",
		Returns: "null",
	})
	RegisterWithGroupAndMeta("deleteAllSessions", rpc.RoleAdmin, adminDeleteAllSessions, &rpc.MethodMeta{
		Name:    "admin:deleteAllSessions",
		Summary: "Delete all sessions",
		Returns: "null",
	})
	RegisterWithGroupAndMeta("getSettings", rpc.RoleAdmin, adminGetSettings, &rpc.MethodMeta{
		Name:    "admin:getSettings",
		Summary: "Get all settings",
		Returns: "object",
	})
	RegisterWithGroupAndMeta("editSettings", rpc.RoleAdmin, adminEditSettings, &rpc.MethodMeta{
		Name:    "admin:editSettings",
		Summary: "Update settings (partial)",
		Returns: "null",
	})
	RegisterWithGroupAndMeta("clearPingRecords", rpc.RoleAdmin, adminClearPingRecords, &rpc.MethodMeta{
		Name: "admin:clearPingRecords", Summary: "Clear all ping records",
	})
	RegisterWithGroupAndMeta("clearAllRecords", rpc.RoleAdmin, adminClearAllRecords, &rpc.MethodMeta{
		Name:    "admin:clearAllRecords",
		Summary: "Delete all load and ping records",
		Returns: "null",
	})
	RegisterWithGroupAndMeta("orderClients", rpc.RoleAdmin, adminOrderClients, &rpc.MethodMeta{
		Name:    "admin:orderClients",
		Summary: "Reorder clients (map of uuid->weight)",
		Returns: "null",
	})
}

func adminGetSessions(ctx context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	ss, err := accounts.GetAllSessions()
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to retrieve sessions: "+err.Error(), nil)
	}
	current := ""
	if meta := rpc.MetaFromContext(ctx); meta != nil {
		current = meta.SessionToken
	}
	return map[string]any{"current": current, "data": ss}, nil
}

func adminDeleteSession(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		Session string `json:"session"`
	}
	req.BindParams(&params)
	if params.Session == "" {
		return nil, rpc.MakeError(rpc.InvalidParams, "session is required", nil)
	}
	if err := accounts.DeleteSession(params.Session); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to delete session: "+err.Error(), nil)
	}
	actor, ip := auditActor(ctx)
	auditlog.Log(ip, actor, "delete session", "info")
	return nil, nil
}

func adminDeleteAllSessions(ctx context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	if err := accounts.DeleteAllSessions(); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to delete all sessions: "+err.Error(), nil)
	}
	actor, ip := auditActor(ctx)
	auditlog.Log(ip, actor, "delete all sessions", "warn")
	return nil, nil
}

func adminGetSettings(_ context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	cst, err := config.GetAll()
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to get settings: "+err.Error(), nil)
	}
	return cst, nil
}

func adminEditSettings(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	cfg := make(map[string]interface{})
	if err := req.BindParams(&cfg); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid or missing request body: "+err.Error(), nil)
	}
	for _, key := range []string{
		"description",
		"site_description",
		"send_ip_addr_to_guest",
		"script_domain",
		"tempory_share_token",
		"tempory_share_token_expire_at",
		"custom_head",
		"custom_body",
		"oauth_enable",
		"oauth_provider",
		"disable_password_login",
		"cors_enabled",
		"cors_allow_origins",
		"cors_origin_check_enabled",
		"cors_allowed_origins",
		"websocket_origin_check",
		"websocket_origin_allowlist",
		"ws_origin_check_enabled",
		"ws_allowed_origins",
		"cloudflared_token",
		"cloudflared_enabled",
		"notification",
		"notification_enabled",
		"metric_store_enabled",
		"metric_db_driver",
		"metric_db_dsn",
		"metric_retention_days",
		"metric_table_prefix",
		"metric_max_open_conns",
		"metric_max_idle_conns",
	} {
		delete(cfg, key)
	}

	if err := config.SetMany(cfg); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to update settings: "+err.Error(), nil)
	}

	message := "update settings: "
	for key := range cfg {
		message += key + ", "
	}
	if len(message) > 2 {
		message = message[:len(message)-2]
	}
	actor, ip := auditActor(ctx)
	auditlog.Log(ip, actor, message, "info")
	return nil, nil
}

func adminClearPingRecords(ctx context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	tasks.DeleteAllPingRecords()
	actor, ip := auditActor(ctx)
	auditlog.Log(ip, actor, "clear ping records", "info")
	return nil, nil
}

func adminClearAllRecords(ctx context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {

	records.DeleteAll()
	tasks.DeleteAllPingRecords()
	actor, ip := auditActor(ctx)
	auditlog.Log(ip, actor, "clear all records", "info")
	return nil, nil
}

func adminOrderClients(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var order map[string]int
	if err := req.BindParams(&order); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid or missing request body: "+err.Error(), nil)
	}
	db := dbcore.GetDBInstance()
	for uuid, weight := range order {
		if err := db.Model(&models.Client{}).Where("uuid = ?", uuid).Update("weight", weight).Error; err != nil {
			return nil, rpc.MakeError(rpc.InternalError, "Failed to update client weight: "+err.Error(), nil)
		}
	}
	actor, ip := auditActor(ctx)
	auditlog.Log(ip, actor, "order clients", "info")
	return nil, nil
}
