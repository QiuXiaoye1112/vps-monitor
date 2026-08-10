package client

import (
	"time"

	"github.com/monitor-monitor/monitor/database/clients"
	"github.com/monitor-monitor/monitor/database/models"
	"github.com/monitor-monitor/monitor/database/tasks"
	report "github.com/monitor-monitor/monitor/protocol/report"
	agent_runtime "github.com/monitor-monitor/monitor/web/agent"
	"github.com/monitor-monitor/monitor/web/realtime"
	report_cache "github.com/monitor-monitor/monitor/web/report"
)

// ingest.go
// agent 上报数据的传输无关处理逻辑。v2 (JSON-RPC) 的 WebSocket 与 HTTP
// 入口经过这里统一落库并更新运行时状态，消除重复。

// ingestReport 保存一次负载上报并刷新运行时状态。
// protocolVersion 保留为运行时能力标记参数，当前仅接受 v2 上报。
// markPresence 为 true 时按 POST 上报会话刷新在线状态（WS 连接自行管理在线状态，应传 false）。
func ingestReport(uuid string, report report.Report, protocolVersion int, markPresence bool) error {
	report.UUID = uuid
	savedReport, err := SaveClientReport(uuid, report)
	if err != nil {
		return err
	}
	if err := clients.UpdateLastReportAt(uuid, savedReport.UpdatedAt); err != nil {
		return err
	}
	agent_runtime.SetLatestReport(uuid, &savedReport)
	agent_runtime.SetClientProtocolVersion(uuid, protocolVersion)
	realtime.Publish(realtime.Event{Kind: realtime.KindStatus, UUID: uuid})
	if markPresence {
		refreshPostPresence(uuid)
	}
	return nil
}

// ingestHistoryReport stores a chart sample without changing the live status,
// presence, or traffic-accounting report cache.
func ingestHistoryReport(uuid string, report report.Report) error {
	report.UUID = uuid
	if err := report_cache.AppendHistoryReport(uuid, report); err != nil {
		return err
	}
	realtime.Publish(realtime.Event{Kind: realtime.KindHistory, UUID: uuid})
	return nil
}

// ingestBasicInfo 保存客户端基础信息。fallbackIP 在上报未携带 IP 时用作兜底。
func ingestBasicInfo(uuid string, info map[string]interface{}, fallbackIP string) error {
	if info == nil {
		info = map[string]interface{}{}
	}
	if err := saveClientBasicInfo(info, uuid, fallbackIP); err != nil {
		return err
	}
	realtime.Publish(realtime.Event{Kind: realtime.KindMetadata, UUID: uuid})
	return nil
}

// ingestPingResult 保存一条 ping 探测结果。
func ingestPingResult(uuid string, taskID uint, value int, finishedAt time.Time) error {
	if err := tasks.SavePingRecord(models.PingRecord{
		Client: uuid,
		TaskId: taskID,
		Value:  value,
		Time:   models.FromTime(finishedAt),
	}); err != nil {
		return err
	}
	realtime.Publish(realtime.Event{
		Kind:   realtime.KindPing,
		UUID:   uuid,
		TaskID: taskID,
		Time:   finishedAt.Format(time.RFC3339Nano),
		Value:  value,
	})
	return nil
}
