package jsonrpc

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/monitor-monitor/monitor/database"
	"github.com/monitor-monitor/monitor/database/clients"
	"github.com/monitor-monitor/monitor/database/dbcore"
	"github.com/monitor-monitor/monitor/database/models"
	"github.com/monitor-monitor/monitor/database/records"
	"github.com/monitor-monitor/monitor/database/tasks"
	"github.com/monitor-monitor/monitor/pkg/config"
	"github.com/monitor-monitor/monitor/pkg/rpc"
	"github.com/monitor-monitor/monitor/protocol/v1"
	"github.com/monitor-monitor/monitor/utils"
	agent_runtime "github.com/monitor-monitor/monitor/web/agent"
	report_cache "github.com/monitor-monitor/monitor/web/report"

	cache "github.com/patrickmn/go-cache"
)

func adjustedTrafficTotals(up, down, compensation int64) (int64, int64) {
	half := compensation / 2
	remainder := compensation % 2
	adjustedUp := up + half + remainder
	adjustedDown := down + half
	if adjustedUp < 0 {
		adjustedDown += adjustedUp
		adjustedUp = 0
	}
	if adjustedDown < 0 {
		adjustedUp += adjustedDown
		adjustedDown = 0
	}
	if adjustedUp < 0 {
		adjustedUp = 0
	}
	return adjustedUp, adjustedDown
}

// pingstats:<uuid>:<enabled-scoped-task-ids>
var pingStatsCache = cache.New(1*time.Minute, 2*time.Minute)
var monthlyTrafficCache = cache.New(cache.NoExpiration, 10*time.Minute)

type pingStat struct {
	Name   string  `json:"name"`
	Latest int     `json:"latest"`
	Avg    int     `json:"avg"`
	Tail   float64 `json:"tail"` // (P99-P50)/P50
	Loss   float64 `json:"loss"` // 丢包率 %
	Min    int     `json:"min"`
	Max    int     `json:"max"`
}

// getPingStatsForNode 计算并缓存节点最近 1 小时 ping 统计
func getPingStatsForNode(uuid string, pingTasks []models.PingTask, taskOrder models.UintArray) map[string]pingStat {
	if uuid == "" {
		return map[string]pingStat{}
	}
	assigned := assignedPingTasksForNode(uuid, pingTasks, taskOrder)
	key := pingStatsCacheKey(uuid, assigned)
	if v, ok := pingStatsCache.Get(key); ok {
		if m, ok2 := v.(map[string]pingStat); ok2 {
			return m
		}
	}
	if len(assigned) == 0 {
		empty := map[string]pingStat{}
		pingStatsCache.Set(key, empty, cache.DefaultExpiration)
		return empty
	}
	end := time.Now()
	start := end.Add(-1 * time.Hour)
	recs, err := tasks.GetPingRecords(uuid, -1, start, end)
	if err != nil || len(recs) == 0 {
		empty := map[string]pingStat{}
		pingStatsCache.Set(key, empty, cache.DefaultExpiration)
		return empty
	}
	grouped := make(map[uint][]models.PingRecord)
	for _, r := range recs {
		for _, t := range assigned {
			if r.TaskId == t.Id {
				grouped[r.TaskId] = append(grouped[r.TaskId], r)
				break
			}
		}
	}
	result := make(map[string]pingStat, len(grouped))
	for _, t := range assigned {
		records := grouped[t.Id]
		if len(records) == 0 {
			continue
		}
		latest := -1
		var latestTs time.Time
		values := make([]int, 0, len(records))
		sum := 0
		valid := 0
		total := 0
		lossCount := 0
		minLat := 0
		maxLat := 0
		for _, r := range records {
			total++
			if r.Value < 0 { // 丢包
				lossCount++
				continue
			}
			values = append(values, r.Value)
			sum += r.Value
			valid++
			if minLat == 0 || r.Value < minLat {
				minLat = r.Value
			}
			if r.Value > maxLat {
				maxLat = r.Value
			}
			ts := r.Time.ToTime()
			if latestTs.IsZero() || ts.After(latestTs) {
				latestTs = ts
				latest = r.Value
			}
		}
		avg := 0
		if valid > 0 {
			avg = sum / valid
		}
		p50, p99 := 0, 0
		if len(values) > 0 {
			sort.Ints(values)
			percentile := func(vals []int, pct float64) int {
				if len(vals) == 0 {
					return 0
				}
				if pct <= 0 {
					return vals[0]
				}
				if pct >= 1 {
					return vals[len(vals)-1]
				}
				pos := (float64(len(vals) - 1)) * pct
				lo := int(math.Floor(pos))
				hi := int(math.Ceil(pos))
				if lo == hi {
					return vals[lo]
				}
				frac := pos - float64(lo)
				v := float64(vals[lo]) + (float64(vals[hi])-float64(vals[lo]))*frac
				return int(math.Round(v))
			}
			p50 = percentile(values, 0.50)
			p99 = percentile(values, 0.99)
		}
		tail := 0.0
		if p50 > 0 && p99 >= p50 {
			tail = float64(p99-p50) / float64(p50)
		}
		lossRate := 0.0
		if total > 0 {
			lossRate = float64(lossCount) / float64(total) * 100
		}
		result[fmt.Sprintf("%d", t.Id)] = pingStat{
			Name:   t.Name,
			Latest: latest,
			Avg:    avg,
			Tail:   tail,
			Loss:   lossRate,
			Min:    minLat,
			Max:    maxLat,
		}
	}
	pingStatsCache.Set(key, result, cache.DefaultExpiration)
	return result
}

func assignedPingTasksForNode(uuid string, pingTasks []models.PingTask, taskOrder models.UintArray) []models.PingTask {
	if uuid == "" {
		return nil
	}
	assigned := make([]models.PingTask, 0, len(pingTasks))
	for _, t := range pingTasks {
		if t.AppliesToClient(uuid) {
			assigned = append(assigned, t)
		}
	}
	if len(taskOrder) == 0 {
		return assigned
	}
	return tasks.OrderPingTasks(taskOrder, assigned)
}

func pingStatsCacheKey(uuid string, assigned []models.PingTask) string {
	taskIDs := make([]string, 0, len(assigned))
	for _, t := range assigned {
		taskIDs = append(taskIDs, fmt.Sprintf("%d", t.Id))
	}
	sort.Strings(taskIDs)
	return fmt.Sprintf("pingstats:%s:%s", uuid, strings.Join(taskIDs, ","))
}

func clearPingStatsCache() {
	pingStatsCache.Flush()
}

func init() {
	RegisterWithGroupAndMeta("getNodes", "common",
		func(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
			return getNodes(ctx, req)
		},
		&rpc.MethodMeta{
			Name:    "getNodes",
			Summary: "Get all nodes",
			Params: []rpc.ParamMeta{
				{
					Name:        "uuid",
					Description: "Specify the UUID of the node",
					Required:    false,
					Type:        "string",
				},
			},
			Returns: "Client | { [uuid]: Client }",
		},
	)
	RegisterWithGroupAndMeta("getNodesLatestStatus", "common",
		func(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
			return getNodesLatestStatus(ctx, req)
		},
		&rpc.MethodMeta{
			Name:    "getNodesLatestStatus",
			Summary: "Get latest status reports (single node or map).",
			Params: []rpc.ParamMeta{
				{
					Name:        "uuid",
					Description: "Specify the UUID of the node (optional)",
					Required:    false,
					Type:        "string",
				},
				{
					Name:        "uuids",
					Description: "Specify multiple UUIDs (array) to get subset (ignored if uuid provided)",
					Required:    false,
					Type:        "string[]",
				},
			},
			Returns: "Record | { [uuid]: Record }",
		},
	)
	Register("getMe", func(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
		return getMe(ctx, req)
	})
	Register("getPublicInfo", getPublicInfo)
	Register("getVersion", getVersion)
	Register("getNodeRecentStatus", getNodeRecentStatus)
}

func getNodes(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	log.Println("[DEBUG_ORDER] common:getNodes was called!")
	var params struct {
		UUID string `json:"uuid"`
	}
	req.BindParams(&params)
	cinfo, err := clients.GetAllClientBasicInfo()
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to get client info", cinfo)
	}
	meta := rpc.MetaFromContext(ctx)

	SendIpAddrToGuest, _ := config.GetAs[bool](config.SendIpAddrToGuestKey)
	if meta.Principal == nil || !meta.Principal.HasRole(rpc.RoleAdmin) {
		// 过滤 Hidden 节点并隐藏敏感字段
		filtered := make([]models.Client, 0, len(cinfo))
		for _, node := range cinfo {
			if node.Hidden { // 非 admin 不显示隐藏节点
				continue
			}
			if SendIpAddrToGuest {
				if node.IPv4 != "" {
					node.IPv4 = strings.Split(node.IPv4, ".")[0] + ".*.*.*"
				}
				if node.IPv6 != "" {
					node.IPv6 = strings.Split(node.IPv6, ":")[0] + ":*:*:*:*:*:*:*"
				}
			} else {
				node.IPv4 = ""
				node.IPv6 = ""
			}

			node.Remark = ""
			node.PublicRemark = ""
			node.Version = ""
			node.Token = ""
			filtered = append(filtered, node)
		}
		cinfo = filtered
	}
	if params.UUID != "" {
		for _, node := range cinfo {
			if node.UUID == params.UUID {
				return node, nil
			}
		}
		return nil, rpc.MakeError(rpc.InvalidParams, "Node not found", params.UUID)
	}

	// 返回以 uuid 为键的字典（每个 value 自身也包含 uuid 字段）
	nodeMap := make(map[string]models.Client, len(cinfo))
	for _, node := range cinfo {
		nodeMap[node.UUID] = node
	}
	return nodeMap, nil
}

func getPublicInfo(_ context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	info, err := database.GetPublicInfo()
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to get public info", err.Error())
	}
	return info, nil
}

func getNodesLatestStatus(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		UUID  string   `json:"uuid"`
		UUIDs []string `json:"uuids"`
	}
	req.BindParams(&params)

	meta := rpc.MetaFromContext(ctx)
	latest := agent_runtime.GetLatestReport() // map[string]*v1.Report (copy)
	onlineUUIDs := agent_runtime.GetAllOnlineUUIDs()
	onlineSet := make(map[string]bool, len(onlineUUIDs))
	for _, uuid := range onlineUUIDs {
		onlineSet[uuid] = true
	}
	cinfo, err := clients.GetAllClientBasicInfo()
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to get client info", err.Error())
	}
	clientByUUID := make(map[string]models.Client, len(cinfo))
	for _, c := range cinfo {
		clientByUUID[c.UUID] = c
	}

	// Hidden 过滤
	if meta.Principal == nil || !meta.Principal.HasRole(rpc.RoleAdmin) {
		hidden := make(map[string]bool, len(cinfo))
		for _, c := range cinfo {
			if c.Hidden {
				hidden[c.UUID] = true
			}
		}
		for uuid := range latest {
			if hidden[uuid] {
				delete(latest, uuid)
			}
		}
	}

	// 如果指定 uuid 但找不到，直接返回 not found
	if params.UUID != "" {
		if _, ok := latest[params.UUID]; !ok {
			return nil, rpc.MakeError(rpc.InvalidParams, "Node not found", params.UUID)
		}
	}

	type recordLike struct {
		Client              string              `json:"client"`
		Time                models.LocalTime    `json:"time"`
		Cpu                 float32             `json:"cpu"`
		Ram                 int64               `json:"ram"`
		RamTotal            int64               `json:"ram_total"`
		Swap                int64               `json:"swap"`
		SwapTotal           int64               `json:"swap_total"`
		Load                float32             `json:"load"`
		Load5               float32             `json:"load5"`
		Load15              float32             `json:"load15"`
		Temp                float32             `json:"temp"`
		Disk                int64               `json:"disk"`
		DiskTotal           int64               `json:"disk_total"`
		NetIn               int64               `json:"net_in"`
		NetOut              int64               `json:"net_out"`
		NetTotalUp          int64               `json:"net_total_up"`
		NetTotalDown        int64               `json:"net_total_down"`
		RawNetTotalUp       int64               `json:"raw_net_total_up"`
		RawNetTotalDown     int64               `json:"raw_net_total_down"`
		MonthlyTraffic      int64               `json:"monthly_traffic"`
		MonthlyTrafficRaw   int64               `json:"monthly_traffic_raw"`
		TrafficCompensation int64               `json:"traffic_compensation"`
		TrafficResetDay     int                 `json:"traffic_reset_day"`
		TrafficResetHour    int                 `json:"traffic_reset_hour"`
		TrafficResetMinute  int                 `json:"traffic_reset_minute"`
		TrafficResetEnabled bool                `json:"traffic_reset_enabled"`
		Process             int                 `json:"process"`
		Connections         int                 `json:"connections"`
		ConnectionsUdp      int                 `json:"connections_udp"`
		Online              bool                `json:"online"`
		Uptime              int64               `json:"uptime"`
		Ping                map[string]pingStat `json:"ping"`
	}

	respMap := make(map[string]recordLike, len(latest))

	// 预取所有 ping 任务
	pingTasks, _ := tasks.GetEnabledPingTasks()

	appendOne := func(uuid string, rep *v1.Report) {
		if rep == nil {
			return
		}
		stats := getPingStatsForNode(uuid, pingTasks, clientByUUID[uuid].PingTaskOrder)
		monthlyUp := rep.Network.TotalUp
		monthlyDown := rep.Network.TotalDown
		var monthly records.MonthlyTraffic
		if client, ok := clientByUUID[uuid]; ok {
			if mt, err := records.CurrentMonthlyTraffic(client, time.Now()); err == nil {
				monthly = mt
				monthlyTrafficCache.Set(uuid, mt, cache.NoExpiration)
				monthlyUp, monthlyDown = adjustedTrafficTotals(
					mt.Up+mt.CarryUp,
					mt.Down+mt.CarryDown,
					mt.Compensation,
				)
			} else if cached, found := monthlyTrafficCache.Get(uuid); found {
				if mt, valid := cached.(records.MonthlyTraffic); valid {
					monthly = mt
					monthlyUp, monthlyDown = adjustedTrafficTotals(
						mt.Up+mt.CarryUp,
						mt.Down+mt.CarryDown,
						mt.Compensation,
					)
				}
				log.Printf("failed to calculate monthly traffic for %s, using last known value: %v", uuid, err)
			} else {
				log.Printf("failed to calculate monthly traffic for %s and no cached value is available: %v", uuid, err)
			}
		}
		resetDay := clientByUUID[uuid].TrafficResetDay
		if !clientByUUID[uuid].TrafficResetEnabled {
			resetDay = 0
		}
		rl := recordLike{
			Client:              uuid,
			Time:                models.FromTime(rep.UpdatedAt),
			Cpu:                 float32(rep.CPU.Usage),
			Ram:                 rep.Ram.Used,
			RamTotal:            rep.Ram.Total,
			Swap:                rep.Swap.Used,
			SwapTotal:           rep.Swap.Total,
			Load:                float32(rep.Load.Load1),
			Load5:               float32(rep.Load.Load5),
			Load15:              float32(rep.Load.Load15),
			Temp:                0,
			Disk:                rep.Disk.Used,
			DiskTotal:           rep.Disk.Total,
			NetIn:               rep.Network.Down,
			NetOut:              rep.Network.Up,
			NetTotalUp:          monthlyUp,
			NetTotalDown:        monthlyDown,
			RawNetTotalUp:       rep.Network.TotalUp,
			RawNetTotalDown:     rep.Network.TotalDown,
			MonthlyTraffic:      monthlyUp + monthlyDown,
			MonthlyTrafficRaw:   monthly.RawTotal,
			TrafficCompensation: monthly.Compensation,
			TrafficResetDay:     resetDay,
			TrafficResetHour:    clientByUUID[uuid].TrafficResetHour,
			TrafficResetMinute:  clientByUUID[uuid].TrafficResetMinute,
			TrafficResetEnabled: clientByUUID[uuid].TrafficResetEnabled,
			Process:             rep.Process,
			Connections:         rep.Connections.TCP,
			ConnectionsUdp:      rep.Connections.UDP,
			Online:              onlineSet[uuid],
			Uptime:              rep.Uptime,
			Ping:                stats,
		}
		respMap[uuid] = rl
	}

	// 选择逻辑
	if params.UUID != "" { // 单个
		appendOne(params.UUID, latest[params.UUID])
		return respMap[params.UUID], nil
	}
	selected := map[string]bool{}
	if len(params.UUIDs) > 0 {
		for _, id := range params.UUIDs {
			selected[id] = true
		}
		for uuid, rep := range latest {
			if selected[uuid] {
				appendOne(uuid, rep)
			}
		}
		return respMap, nil
	}
	for uuid, rep := range latest {
		appendOne(uuid, rep)
	}
	return respMap, nil
}

func applyTrafficCompensation(up, down, compensation int64) (int64, int64) {
	half := compensation / 2
	rem := compensation % 2
	return up + half + rem, down + half
}

func getMe(ctx context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var resp struct {
		TwoFAEnabled bool   `json:"2fa_enabled"`
		LoggedIn     bool   `json:"logged_in"`
		SSOId        string `json:"sso_id"`
		SSOType      string `json:"sso_type"`
		Username     string `json:"username"`
		UUID         string `json:"uuid"`
	}

	meta := rpc.MetaFromContext(ctx)

	switch meta.Principal.Type {
	case rpc.PrincipalUser, rpc.PrincipalAPIKey:
		if meta.User == nil {
			resp.LoggedIn = true
			resp.Username = "api_key"
			return resp, nil
		}
		resp.TwoFAEnabled = meta.User.TwoFactor != ""
		resp.LoggedIn = true
		resp.SSOId = meta.User.SSOID
		resp.SSOType = meta.User.SSOType
		resp.Username = meta.User.Username
		resp.UUID = meta.User.UUID
		return resp, nil
	case rpc.PrincipalAnonymous:
		resp.LoggedIn = false
		return resp, nil
	case rpc.PrincipalAgent:
		resp.LoggedIn = true
		resp.SSOId = "client"
		resp.SSOType = "client"
		resp.Username = "client"
		resp.UUID = meta.ClientToken
		client, err := clients.GetClientUUIDByToken(meta.ClientToken)
		if err != nil {
			resp.UUID = client
		}
		return resp, nil
	default:
		resp.LoggedIn = false
		return resp, nil
	}
}

func getVersion(_ context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	return struct {
		Version string `json:"version"`
		Hash    string `json:"hash"`
	}{
		Version: utils.CurrentVersion,
		Hash:    utils.VersionHash,
	}, nil
}

func getNodeRecentStatus(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		UUID string `json:"uuid"`
	}
	req.BindParams(&params)
	if params.UUID == "" {
		return nil, rpc.MakeError(rpc.InvalidParams, "UUID is required", params)
	}
	meta := rpc.MetaFromContext(ctx)
	// 登录状态检查
	isLogin := false
	if meta.Principal != nil && meta.Principal.HasRole(rpc.RoleAdmin) {
		isLogin = true
	}

	// 仅在未登录时需要 Hidden 信息做过滤
	hiddenMap := map[string]bool{}
	if !isLogin {
		var hiddenClients []models.Client
		db := dbcore.GetDBInstance()
		_ = db.Select("uuid").Where("hidden = ?", true).Find(&hiddenClients).Error
		for _, cli := range hiddenClients {
			hiddenMap[cli.UUID] = true
		}

		if hiddenMap[params.UUID] {
			return nil, rpc.MakeError(rpc.InvalidParams, "UUID is required", params) //防止未登录用户获取隐藏客户端数据
		}
	}

	raw, _ := report_cache.Records.Get(params.UUID)
	reports, _ := raw.([]v1.Report)

	// 扁平化为 { count, records: [] }
	type flatRecord struct {
		Client         string           `json:"client"`
		Time           models.LocalTime `json:"time"`
		Cpu            float32          `json:"cpu"`
		Ram            int64            `json:"ram"`
		RamTotal       int64            `json:"ram_total"`
		Swap           int64            `json:"swap"`
		SwapTotal      int64            `json:"swap_total"`
		Load           float32          `json:"load"`
		Temp           float32          `json:"temp"`
		Disk           int64            `json:"disk"`
		DiskTotal      int64            `json:"disk_total"`
		NetIn          int64            `json:"net_in"`
		NetOut         int64            `json:"net_out"`
		NetTotalUp     int64            `json:"net_total_up"`
		NetTotalDown   int64            `json:"net_total_down"`
		Process        int              `json:"process"`
		Connections    int              `json:"connections"`
		ConnectionsUdp int              `json:"connections_udp"`
	}

	resp := struct {
		Count   int          `json:"count"`
		Records []flatRecord `json:"records"`
	}{
		Count:   0,
		Records: []flatRecord{},
	}

	if len(reports) == 0 {
		return resp, nil
	}

	resp.Records = make([]flatRecord, 0, len(reports))
	for _, r := range reports {
		fr := flatRecord{
			Client:         params.UUID,
			Time:           models.FromTime(r.UpdatedAt),
			Cpu:            float32(r.CPU.Usage),
			Ram:            r.Ram.Used,
			RamTotal:       r.Ram.Total,
			Swap:           r.Swap.Used,
			SwapTotal:      r.Swap.Total,
			Load:           float32(r.Load.Load1),
			Temp:           0,
			Disk:           r.Disk.Used,
			DiskTotal:      r.Disk.Total,
			NetIn:          r.Network.Down,
			NetOut:         r.Network.Up,
			NetTotalUp:     r.Network.TotalUp,
			NetTotalDown:   r.Network.TotalDown,
			Process:        r.Process,
			Connections:    r.Connections.TCP,
			ConnectionsUdp: r.Connections.UDP,
		}
		resp.Records = append(resp.Records, fr)
	}
	resp.Count = len(resp.Records)
	return resp, nil
}
