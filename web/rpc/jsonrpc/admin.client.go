package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/monitor-monitor/monitor/database/auditlog"
	"github.com/monitor-monitor/monitor/database/clients"
	"github.com/monitor-monitor/monitor/database/records"
	"github.com/monitor-monitor/monitor/database/tasks"
	"github.com/monitor-monitor/monitor/pkg/rpc"
	v2 "github.com/monitor-monitor/monitor/protocol/v2"
	agent_runtime "github.com/monitor-monitor/monitor/web/agent"
	report_cache "github.com/monitor-monitor/monitor/web/report"
)

// admin.client.go
// client 资源的 RPC2 方法（admin 命名空间）。承载原 web/api/admin/client.go 的业务逻辑，
// 包含审计日志与运行时副作用。传统 REST handler 经 CallFromGin 转调这些方法。

func init() {
	RegisterWithGroupAndMeta("addClient", rpc.RoleAdmin, adminAddClient, &rpc.MethodMeta{
		Name:    "admin:addClient",
		Summary: "Create a new client",
		Params: []rpc.ParamMeta{
			{Name: "name", Type: "string", Required: false, Description: "Optional client name"},
			{Name: "group", Type: "string", Required: false, Description: "Optional client group"},
		},
		Returns: "{ uuid: string, token: string }",
	})
	RegisterWithGroupAndMeta("editClient", rpc.RoleAdmin, adminEditClient, &rpc.MethodMeta{
		Name:    "admin:editClient",
		Summary: "Edit a client (partial update)",
		Params: []rpc.ParamMeta{
			{Name: "uuid", Type: "string", Required: true, Description: "Client UUID"},
		},
		Returns: "null",
	})
	RegisterWithGroupAndMeta("removeClient", rpc.RoleAdmin, adminRemoveClient, &rpc.MethodMeta{
		Name:    "admin:removeClient",
		Summary: "Delete a client",
		Params: []rpc.ParamMeta{
			{Name: "uuid", Type: "string", Required: true, Description: "Client UUID"},
		},
		Returns: "null",
	})
	RegisterWithGroupAndMeta("getClient", rpc.RoleAdmin, adminGetClient, &rpc.MethodMeta{
		Name:    "admin:getClient",
		Summary: "Get a client by UUID",
		Params: []rpc.ParamMeta{
			{Name: "uuid", Type: "string", Required: true, Description: "Client UUID"},
		},
		Returns: "Client",
	})
	RegisterWithGroupAndMeta("listClients", rpc.RoleAdmin, adminListClients, &rpc.MethodMeta{
		Name:    "admin:listClients",
		Summary: "List all clients (basic info)",
		Returns: "Client[]",
	})
	RegisterWithGroupAndMeta("getClientToken", rpc.RoleAdmin, adminGetClientToken, &rpc.MethodMeta{
		Name:    "admin:getClientToken",
		Summary: "Get a client's token by UUID",
		Params: []rpc.ParamMeta{
			{Name: "uuid", Type: "string", Required: true, Description: "Client UUID"},
		},
		Returns: "{ token: string }",
	})
	RegisterWithGroupAndMeta("clearRecords", rpc.RoleAdmin, adminClearRecords, &rpc.MethodMeta{
		Name:    "admin:clearRecords",
		Summary: "Delete all load records",
		Returns: "null",
	})
	RegisterWithGroupAndMeta("resetClientTraffic", rpc.RoleAdmin, adminResetClientTraffic, &rpc.MethodMeta{
		Name:    "admin:resetClientTraffic",
		Summary: "Reset one client's cumulative traffic and compensation",
		Params: []rpc.ParamMeta{
			{Name: "uuid", Type: "string", Required: true, Description: "Client UUID"},
		},
		Returns: "null",
	})
}

// auditActor 从上下文提取审计用的 actor UUID 与来源 IP。
func auditActor(ctx context.Context) (uuid, ip string) {
	if meta := rpc.MetaFromContext(ctx); meta != nil {
		uuid = meta.UserUUID
		ip = meta.RemoteIP
	}
	return uuid, ip
}

func adminAddClient(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	raw := map[string]interface{}{}
	if err := req.BindParams(&raw); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid params", nil)
	}
	name, _ := raw["name"].(string)
	group, _ := raw["group"].(string)
	pingTaskOrder, hasPingTaskOrder, err := popPingTaskOrder(raw)
	if err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, err.Error(), nil)
	}

	var (
		uuid, token string
		createErr   error
	)
	if name == "" && group == "" {
		uuid, token, createErr = clients.CreateClient()
	} else {
		uuid, token, createErr = clients.CreateClientWithNameAndGroup(name, group)
	}
	if createErr != nil {
		return nil, rpc.MakeError(rpc.InternalError, createErr.Error(), nil)
	}
	update := clientCreateUpdateFromParams(uuid, raw)
	if len(update) > 1 {
		if err := clients.SaveClient(update); err != nil {
			return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
		}
	}
	if hasPingTaskOrder {
		if err := tasks.SetClientPingTaskOrder(uuid, pingTaskOrder); err != nil {
			_ = clients.DeleteClient(uuid)
			return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
		}
		clearPingStatsCache()
	}
	if name != "" {
		actor, ip := auditActor(ctx)
		auditlog.Log(ip, actor, "create client:"+uuid, "info")
	}
	return map[string]any{"uuid": uuid, "token": token}, nil
}

func clientCreateUpdateFromParams(uuid string, raw map[string]interface{}) map[string]interface{} {
	update := map[string]interface{}{"uuid": uuid}
	for _, key := range []string{
		"name",
		"group",
		"hidden",
		"traffic_limit",
		"traffic_limit_type",
		"traffic_reset_day",
		"traffic_reset_hour",
		"traffic_reset_minute",
		"traffic_compensation",
		"traffic_reset_enabled",
	} {
		value, ok := raw[key]
		if !ok {
			continue
		}
		update[key] = normalizeClientCreateValue(key, value)
	}
	return update
}

func normalizeClientCreateValue(key string, value interface{}) interface{} {
	switch key {
	case "traffic_reset_day", "traffic_reset_hour", "traffic_reset_minute":
		if n, ok := asInt64(value); ok {
			return int(n)
		}
	case "traffic_limit", "traffic_compensation":
		if n, ok := asInt64(value); ok {
			return n
		}
	}
	return value
}

func asInt64(value interface{}) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int8:
		return int64(v), true
	case int16:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case uint:
		return int64(v), true
	case uint8:
		return int64(v), true
	case uint16:
		return int64(v), true
	case uint32:
		return int64(v), true
	case uint64:
		if v > uint64(^uint64(0)>>1) {
			return 0, false
		}
		return int64(v), true
	case float32:
		return int64(v), true
	case float64:
		return int64(v), true
	case json.Number:
		n, err := v.Int64()
		if err == nil {
			return n, true
		}
		f, err := v.Float64()
		if err != nil {
			return 0, false
		}
		return int64(f), true
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return n, err == nil
	default:
		return 0, false
	}
}

func adminEditClient(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var update map[string]interface{}
	if err := req.BindParams(&update); err != nil || update == nil {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid params", nil)
	}
	for _, key := range []string{
		"token",
		"remark",
		"public_remark",
		"price",
		"billing_cycle",
		"auto_renewal",
		"currency",
		"expired_at",
	} {
		delete(update, key)
	}
	uuid, _ := update["uuid"].(string)
	if uuid == "" {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid or missing UUID", nil)
	}
	pingTaskOrder, hasPingTaskOrder, err := popPingTaskOrder(update)
	if err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, err.Error(), nil)
	}
	delete(update, "traffic_compensation_base")
	if err := clients.SaveClient(update); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
	}
	if hasPingTaskOrder {
		if err := tasks.SetClientPingTaskOrder(uuid, pingTaskOrder); err != nil {
			return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
		}
		clearPingStatsCache()
	}
	actor, ip := auditActor(ctx)
	auditlog.Log(ip, actor, "edit client:"+uuid, "info")
	return nil, nil
}

func popPingTaskOrder(update map[string]interface{}) ([]uint, bool, error) {
	raw, exists := update["ping_task_order"]
	delete(update, "ping_task_order")
	if !exists {
		return nil, false, nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, true, fmt.Errorf("invalid ping_task_order")
	}
	var order []uint
	if err := json.Unmarshal(encoded, &order); err != nil {
		return nil, true, fmt.Errorf("invalid ping_task_order")
	}
	return order, true, nil
}

func adminRemoveClient(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		UUID string `json:"uuid"`
	}
	req.BindParams(&params)
	if params.UUID == "" {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid or missing UUID", nil)
	}
	report_cache.DeleteClientReports(params.UUID)
	if err := clients.DeleteClient(params.UUID); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to delete client"+err.Error(), nil)
	}
	actor, ip := auditActor(ctx)
	auditlog.Log(ip, actor, "delete client:"+params.UUID, "warn")
	agent_runtime.DeleteConnectedClients(params.UUID)
	agent_runtime.DeleteLatestReport(params.UUID)
	return nil, nil
}

func adminGetClient(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		UUID string `json:"uuid"`
	}
	req.BindParams(&params)
	if params.UUID == "" {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid or missing UUID", nil)
	}
	result, err := clients.GetClientByUUID(params.UUID)
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
	}
	result.Token = ""
	result.Remark = ""
	result.PublicRemark = ""
	return result, nil
}

func adminListClients(_ context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	cls, err := clients.GetAllClientBasicInfo()
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
	}
	return cls, nil
}

func adminGetClientToken(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		UUID string `json:"uuid"`
	}
	req.BindParams(&params)
	if params.UUID == "" {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid or missing UUID", nil)
	}
	token, err := clients.GetClientTokenByUUID(params.UUID)
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
	}
	return map[string]any{"token": token}, nil
}

func adminClearRecords(ctx context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	if err := records.DeleteAll(); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to delete Record"+err.Error(), nil)
	}
	actor, ip := auditActor(ctx)
	auditlog.Log(ip, actor, "clear records", "warn")
	return nil, nil
}

func adminResetClientTraffic(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		UUID string `json:"uuid"`
	}
	if err := req.BindParams(&params); err != nil || strings.TrimSpace(params.UUID) == "" {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid or missing UUID", nil)
	}
	params.UUID = strings.TrimSpace(params.UUID)
	var snapshot v2.TrafficSnapshotResultParams
	var clearedAt time.Time
	err := report_cache.WithTrafficPersistencePaused(params.UUID, func() error {
		var err error
		snapshot, err = agent_runtime.RequestTrafficSnapshot(params.UUID, 12*time.Second)
		if err != nil {
			return err
		}
		if _, err = time.Parse(time.RFC3339Nano, snapshot.CapturedAt); err != nil {
			return fmt.Errorf("Agent 返回的流量快照时间无效")
		}
		// Record history uses center receive times, so the center clock is the
		// authoritative database boundary. Exact bytes come from the snapshot.
		clearedAt = time.Now()
		return clients.ResetTrafficAccounting(
			params.UUID,
			clearedAt,
			snapshot.TotalUp,
			snapshot.TotalDown,
		)
	})
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "流量清零失败："+err.Error(), nil)
	}
	monthlyTrafficCache.Delete(params.UUID)
	actor, ip := auditActor(ctx)
	auditlog.Log(ip, actor, "reset client traffic:"+params.UUID, "warn")
	return nil, nil
}
