package agent

import (
	"strconv"
	"strings"
	"sync"
	"time"

	report "github.com/monitor-monitor/monitor/protocol/report"
	"github.com/monitor-monitor/monitor/web/connection"
	"github.com/monitor-monitor/monitor/web/realtime"
)

var (
	connectedClients  = make(map[string]*connection.SafeConn)
	connectedClientV2 = make(map[string]bool)
	latestReport      = make(map[string]*report.Report)
	// presenceOnly stores online state for non-WebSocket agents (e.g., Nezha gRPC)
	// value keeps connectionID and a soft expiration to avoid flicker
	presenceOnly = make(map[string]struct {
		id     int64
		expire time.Time
	})
	mu = sync.RWMutex{}
)

func GetConnectedClients() map[string]*connection.SafeConn {
	mu.RLock()
	defer mu.RUnlock()
	clientsCopy := make(map[string]*connection.SafeConn)
	for k, v := range connectedClients {
		clientsCopy[k] = v
	}
	return clientsCopy
}

func SetConnectedClients(uuid string, conn *connection.SafeConn) {
	mu.Lock()
	defer mu.Unlock()
	connectedClients[uuid] = conn
}

func SetClientProtocolVersion(uuid string, version int) {
	mu.Lock()
	defer mu.Unlock()
	connectedClientV2[uuid] = version >= 2
}

func IsV2Client(uuid string) bool {
	mu.RLock()
	defer mu.RUnlock()
	return connectedClientV2[uuid]
}

func DeleteClientConditionally(uuid string, connToRemove *connection.SafeConn) {
	mu.Lock()
	// 检查当前 map 里的 conn 是否就是要删除的这一个
	if currentConn, exists := connectedClients[uuid]; exists && currentConn == connToRemove {
		delete(connectedClients, uuid)
		delete(connectedClientV2, uuid)
		mu.Unlock()
		realtime.Publish(realtime.Event{Kind: realtime.KindStatus, UUID: uuid})
		return
	}
	mu.Unlock()
}
func DeleteConnectedClients(uuid string) {
	mu.Lock()
	conn := connectedClients[uuid]
	delete(connectedClients, uuid)
	delete(connectedClientV2, uuid)
	delete(presenceOnly, uuid)
	mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
	realtime.Publish(realtime.Event{Kind: realtime.KindStatus, UUID: uuid})
}

// SetPresence sets or clears presence for non-WebSocket agents.
// When present=false, it only clears if the connectionID matches current one.
// KeepAlivePresence sets presence with TTL for non-WebSocket agents.
func KeepAlivePresence(uuid string, connectionID int64, ttl time.Duration) {
	mu.Lock()
	defer mu.Unlock()
	presenceOnly[uuid] = struct {
		id     int64
		expire time.Time
	}{id: connectionID, expire: time.Now().Add(ttl)}
}

var defaultPresenceTTL = 20 * time.Second

// SetPresence keeps compatibility with existing callers.
func SetPresence(uuid string, connectionID int64, present bool) {
	mu.Lock()
	if present {
		presenceOnly[uuid] = struct {
			id     int64
			expire time.Time
		}{id: connectionID, expire: time.Now().Add(defaultPresenceTTL)}
		mu.Unlock()
		realtime.Publish(realtime.Event{Kind: realtime.KindStatus, UUID: uuid})
		return
	}
	if cur, ok := presenceOnly[uuid]; ok && cur.id == connectionID {
		delete(presenceOnly, uuid)
		mu.Unlock()
		realtime.Publish(realtime.Event{Kind: realtime.KindStatus, UUID: uuid})
		return
	}
	mu.Unlock()
}

// GetAllOnlineUUIDs returns a de-duplicated list of online UUIDs from both WebSocket and non-WebSocket agents.
func GetAllOnlineUUIDs() []string {
	mu.RLock()
	defer mu.RUnlock()
	set := make(map[string]struct{})
	for k := range connectedClients {
		set[k] = struct{}{}
	}
	now := time.Now()
	for k, v := range presenceOnly {
		if v.expire.After(now) {
			set[k] = struct{}{}
		}
	}
	res := make([]string, 0, len(set))
	for k := range set {
		res = append(res, k)
	}
	return res
}
func GetLatestReport() map[string]*report.Report {
	mu.RLock()
	defer mu.RUnlock()
	reportCopy := make(map[string]*report.Report)
	for k, v := range latestReport {
		if v == nil {
			continue
		}
		copy := *v
		reportCopy[k] = &copy
	}
	return reportCopy
}
func ShouldAcceptReport(uuid string, incoming *report.Report) bool {
	mu.RLock()
	defer mu.RUnlock()
	return isNewerReport(latestReport[uuid], incoming)
}

func SetLatestReport(uuid string, incoming *report.Report) bool {
	if incoming == nil {
		return false
	}
	mu.Lock()
	defer mu.Unlock()
	if !isNewerReport(latestReport[uuid], incoming) {
		return false
	}
	copy := *incoming
	latestReport[uuid] = &copy
	return true
}

func isNewerReport(current, incoming *report.Report) bool {
	if incoming == nil {
		return false
	}
	if current == nil {
		return true
	}
	currentNetwork := current.Network
	incomingNetwork := incoming.Network
	if currentNetwork.LedgerEpoch != "" || incomingNetwork.LedgerEpoch != "" {
		if incomingNetwork.LedgerEpoch != currentNetwork.LedgerEpoch {
			if incomingNetwork.LedgerEpoch == "" {
				return false
			}
			if currentNetwork.LedgerEpoch == "" {
				return true
			}
			incomingEpoch, incomingEpochOK := ledgerEpochTime(incomingNetwork.LedgerEpoch)
			currentEpoch, currentEpochOK := ledgerEpochTime(currentNetwork.LedgerEpoch)
			if incomingEpochOK && currentEpochOK && incomingEpoch != currentEpoch {
				return incomingEpoch > currentEpoch
			}
			if !incomingNetwork.CapturedAt.IsZero() && !currentNetwork.CapturedAt.IsZero() {
				return incomingNetwork.CapturedAt.After(currentNetwork.CapturedAt)
			}
			// A new epoch indicates a rebuilt ledger. Accept it when capture
			// timestamps are unavailable, then reject the previous epoch.
			return true
		}
	}
	if currentNetwork.CycleGeneration != 0 || incomingNetwork.CycleGeneration != 0 {
		if incomingNetwork.CycleGeneration != currentNetwork.CycleGeneration {
			return incomingNetwork.CycleGeneration > currentNetwork.CycleGeneration
		}
		if currentNetwork.SampleSequence != 0 || incomingNetwork.SampleSequence != 0 {
			if incomingNetwork.SampleSequence != currentNetwork.SampleSequence {
				return incomingNetwork.SampleSequence > currentNetwork.SampleSequence
			}
			if !incomingNetwork.CapturedAt.IsZero() || !currentNetwork.CapturedAt.IsZero() {
				return incomingNetwork.CapturedAt.After(currentNetwork.CapturedAt)
			}
			return false
		}
	}

	incomingStart, incomingStartErr := time.Parse(time.RFC3339Nano, incomingNetwork.CycleStartedAt)
	currentStart, currentStartErr := time.Parse(time.RFC3339Nano, currentNetwork.CycleStartedAt)
	incomingStartOK := incomingStartErr == nil
	currentStartOK := currentStartErr == nil
	if incomingStartOK && currentStartOK && !incomingStart.Equal(currentStart) {
		return incomingStart.After(currentStart)
	}
	if !incomingNetwork.CapturedAt.IsZero() || !currentNetwork.CapturedAt.IsZero() {
		return incomingNetwork.CapturedAt.After(currentNetwork.CapturedAt)
	}
	// Old agents do not provide ordering metadata. Preserve their existing
	// behavior while newer agents are protected by generation and sequence.
	return true
}

func ledgerEpochTime(epoch string) (int64, bool) {
	prefix, _, found := strings.Cut(epoch, "-")
	if !found {
		prefix = epoch
	}
	value, err := strconv.ParseInt(prefix, 10, 64)
	return value, err == nil
}
func DeleteLatestReport(uuid string) {
	mu.Lock()
	defer mu.Unlock()
	delete(latestReport, uuid)
}
