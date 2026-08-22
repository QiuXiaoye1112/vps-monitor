package v2

import report "github.com/monitor-monitor/monitor/protocol/report"

const (
	Version                          = "2.0"
	MethodAgentReport                = "agent.report"
	MethodAgentHistory               = "agent.historyReport"
	MethodAgentBasicInfo             = "agent.basicInfo"
	MethodAgentPingResult            = "agent.pingResult"
	MethodAgentTaskResult            = "agent.taskResult"
	MethodAgentExec                  = "agent.exec"
	MethodAgentPing                  = "agent.ping"
	MethodAgentMessage               = "agent.message"
	MethodAgentEvent                 = "agent.event"
	MethodAgentTerminal              = "agent.terminal.request"
	MethodAgentFile                  = "agent.file.request"
	MethodAgentPull                  = "agent.pull"
	MethodAgentTrafficSnapshot       = "agent.trafficSnapshot"
	MethodAgentTrafficSnapshotResult = "agent.trafficSnapshotResult"
	MethodAgentTrafficConfig         = "agent.trafficConfig"
	MethodAgentTrafficReset          = "agent.trafficReset"
	MethodAgentTrafficResetResult    = "agent.trafficResetResult"
)

type Request struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
	ID      any    `json:"id,omitempty"`
	EventID string `json:"event_id,omitempty"`
}

type Response struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *RPCError `json:"error,omitempty"`
}

type Event struct {
	ID        string `json:"id"`
	Method    string `json:"method"`
	Params    any    `json:"params,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type ReportParams struct {
	Report      report.Report `json:"report"`
	AckEventIDs []string      `json:"ack_event_ids,omitempty"`
}

type BasicInfoParams struct {
	Info map[string]interface{} `json:"info"`
}

type PingResultParams struct {
	TaskID     uint   `json:"task_id"`
	PingType   string `json:"ping_type"`
	Value      int    `json:"value"`
	FinishedAt string `json:"finished_at"`
}

type PullParams struct {
	Capabilities []string `json:"capabilities,omitempty"`
	AckEventIDs  []string `json:"ack_event_ids,omitempty"`
	LastEventID  string   `json:"last_event_id,omitempty"`
}

type ExecParams struct {
	TaskID  string `json:"task_id"`
	Command string `json:"command"`
}

type PingParams struct {
	TaskID uint   `json:"ping_task_id"`
	Type   string `json:"ping_type"`
	Target string `json:"ping_target"`
}

type MessageParams struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type EventParams struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

type TerminalRequestParams struct {
	RequestID string `json:"request_id"`
}

type FileRequestParams struct {
	RequestID string `json:"request_id"`
}

type TrafficSnapshotParams struct {
	OperationID string `json:"operation_id"`
}

type TrafficSnapshotResultParams struct {
	OperationID    string `json:"operation_id"`
	CapturedAt     string `json:"captured_at"`
	CycleID        string `json:"cycle_id"`
	CycleStartedAt string `json:"cycle_started_at"`
	TotalUp        int64  `json:"total_up"`
	TotalDown      int64  `json:"total_down"`
}

type TrafficConfigParams struct {
	Enabled  bool   `json:"enabled"`
	Day      int    `json:"day"`
	Hour     int    `json:"hour"`
	Minute   int    `json:"minute"`
	Timezone string `json:"timezone"`
}

type TrafficResetParams struct {
	OperationID string `json:"operation_id"`
}

type TrafficResetResultParams = TrafficSnapshotResultParams

func Success(id any, result any) Response {
	return Response{JSONRPC: Version, ID: id, Result: result}
}

func Error(id any, code int, message string, data any) Response {
	return Response{JSONRPC: Version, ID: id, Error: &RPCError{Code: code, Message: message, Data: data}}
}
