package jsonrpc

import (
	"math"
	"testing"

	"github.com/monitor-monitor/monitor/database/models"
	reportmodel "github.com/monitor-monitor/monitor/protocol/report"
)

func TestTrafficAdjustmentIsScopedToCycleAndGeneration(t *testing.T) {
	client := models.Client{
		TrafficAdjustmentUp:         200,
		TrafficAdjustmentDown:       -50,
		TrafficAdjustmentCycleID:    "2026-08-01T00:00:00Z",
		TrafficAdjustmentGeneration: 3,
	}
	report := &reportmodel.Report{Network: reportmodel.NetworkReport{
		CycleID:         "2026-08-01T00:00:00Z",
		CycleGeneration: 3,
		TotalUp:         1000,
		TotalDown:       100,
	}}
	if !trafficAdjustmentApplies(client, report) {
		t.Fatal("matching traffic adjustment was not applied")
	}
	if got := adjustedTrafficValue(report.Network.TotalUp, client.TrafficAdjustmentUp); got != 1200 {
		t.Fatalf("adjusted up = %d", got)
	}
	if got := adjustedTrafficValue(report.Network.TotalDown, client.TrafficAdjustmentDown); got != 50 {
		t.Fatalf("adjusted down = %d", got)
	}

	report.Network.CycleGeneration = 4
	if trafficAdjustmentApplies(client, report) {
		t.Fatal("stale generation adjustment was applied")
	}
}

func TestAdjustedTrafficValueClampsBounds(t *testing.T) {
	if got := adjustedTrafficValue(10, -20); got != 0 {
		t.Fatalf("negative adjusted traffic = %d", got)
	}
	if got := adjustedTrafficValue(math.MaxInt64-1, 10); got != math.MaxInt64 {
		t.Fatalf("overflow adjusted traffic = %d", got)
	}
}
