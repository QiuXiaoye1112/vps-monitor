package agent

import (
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
	// 只从 map 中删除，不再负责关闭连接
	delete(connectedClients, uuid)
	delete(connectedClientV2, uuid)
	mu.Unlock()
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
		reportCopy[k] = v
	}
	return reportCopy
}
func SetLatestReport(uuid string, report *report.Report) {
	mu.Lock()
	defer mu.Unlock()
	latestReport[uuid] = report
}
func DeleteLatestReport(uuid string) {
	mu.Lock()
	defer mu.Unlock()
	delete(latestReport, uuid)
}
