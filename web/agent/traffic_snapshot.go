package agent

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	v2 "github.com/monitor-monitor/monitor/protocol/v2"
)

var (
	trafficSnapshotMu       sync.Mutex
	pendingTrafficSnapshots = make(map[string]pendingTrafficSnapshot)
)

var ErrTrafficSnapshotNotPending = errors.New("traffic snapshot operation is no longer pending")

type pendingTrafficSnapshot struct {
	clientUUID string
	result     chan v2.TrafficSnapshotResultParams
}

// RequestTrafficSnapshot asks an online v2 Agent for an immediate counter
// sample. The operation is intentionally synchronous: strict clearing must not
// report success until the counter boundary has actually been captured.
func RequestTrafficSnapshot(uuid string, timeout time.Duration) (v2.TrafficSnapshotResultParams, error) {
	return requestTrafficOperation(uuid, timeout, v2.MethodAgentTrafficSnapshot)
}

// RequestTrafficReset asks the Agent to atomically reset its local traffic
// ledger. The center does not maintain a second baseline or cumulative ledger.
func RequestTrafficReset(uuid string, timeout time.Duration) (v2.TrafficResetResultParams, error) {
	return requestTrafficOperation(uuid, timeout, v2.MethodAgentTrafficReset)
}

func requestTrafficOperation(uuid string, timeout time.Duration, method string) (v2.TrafficSnapshotResultParams, error) {
	if !IsV2Client(uuid) || !IsAgentOnline(uuid) {
		return v2.TrafficSnapshotResultParams{}, errors.New("Agent 离线或版本过低，无法严格清零")
	}

	operationID := newV2EventID()
	pending := pendingTrafficSnapshot{
		clientUUID: uuid,
		result:     make(chan v2.TrafficSnapshotResultParams, 1),
	}
	trafficSnapshotMu.Lock()
	pendingTrafficSnapshots[operationID] = pending
	trafficSnapshotMu.Unlock()
	defer func() {
		trafficSnapshotMu.Lock()
		delete(pendingTrafficSnapshots, operationID)
		trafficSnapshotMu.Unlock()
	}()

	var params any = v2.TrafficSnapshotParams{OperationID: operationID}
	if method == v2.MethodAgentTrafficReset {
		params = v2.TrafficResetParams{OperationID: operationID}
	}
	if !DispatchV2Event(uuid, method, params) {
		return v2.TrafficSnapshotResultParams{}, errors.New("Agent 不支持严格流量清零")
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case result := <-pending.result:
		return result, nil
	case <-timer.C:
		return v2.TrafficSnapshotResultParams{}, errors.New("等待 Agent 流量快照超时，未执行清零")
	}
}

// ResolveTrafficSnapshot delivers a snapshot notification to the matching
// admin request. Unknown/late operation IDs are rejected rather than being
// allowed to clear an unrelated or newer ledger.
func ResolveTrafficSnapshot(uuid string, result v2.TrafficSnapshotResultParams) error {
	result.OperationID = strings.TrimSpace(result.OperationID)
	if result.OperationID == "" || result.TotalUp < 0 || result.TotalDown < 0 {
		return errors.New("invalid traffic snapshot result")
	}

	trafficSnapshotMu.Lock()
	pending, ok := pendingTrafficSnapshots[result.OperationID]
	trafficSnapshotMu.Unlock()
	if !ok {
		return ErrTrafficSnapshotNotPending
	}
	if pending.clientUUID != uuid {
		return fmt.Errorf("traffic snapshot client mismatch")
	}

	select {
	case pending.result <- result:
	default:
	}
	return nil
}

func ResolveTrafficReset(uuid string, result v2.TrafficResetResultParams) error {
	return ResolveTrafficSnapshot(uuid, result)
}
