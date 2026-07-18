package jsonrpc

import (
	"testing"
	"time"

	"github.com/monitor-monitor/monitor/database/models"
)

func TestClampPingRecordRangeLimitsWindowToSevenDays(t *testing.T) {
	end := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	start := end.Add(-10 * 24 * time.Hour)

	gotStart, gotEnd := clampPingRecordRange(start, end)

	if !gotEnd.Equal(end) {
		t.Fatalf("end time changed: got %s, want %s", gotEnd, end)
	}
	if got := gotEnd.Sub(gotStart); got != maxPingRecordWindow {
		t.Fatalf("window = %s, want %s", got, maxPingRecordWindow)
	}
}

func TestClampPingRecordRangeKeepsShortWindow(t *testing.T) {
	end := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	start := end.Add(-3 * time.Hour)

	gotStart, gotEnd := clampPingRecordRange(start, end)

	if !gotStart.Equal(start) || !gotEnd.Equal(end) {
		t.Fatalf("range changed: got %s..%s, want %s..%s", gotStart, gotEnd, start, end)
	}
}

func TestFilterPingRecordsByScopedTasks(t *testing.T) {
	pingTasks := []models.PingTask{
		{Id: 1, Enabled: true, DefaultOn: true},
		{Id: 2, Enabled: false, DefaultOn: true},
		{Id: 3, Enabled: true, Clients: models.StringArray{"node-b"}},
		{Id: 4, Enabled: true, Clients: models.StringArray{"node-a"}},
	}
	scoped := scopedPingTasksForRecords(pingTasks, -1, "", nil)
	recs := []models.PingRecord{
		{Client: "node-a", TaskId: 1, Value: 100},
		{Client: "node-a", TaskId: 2, Value: 200},
		{Client: "node-a", TaskId: 3, Value: 300},
		{Client: "node-a", TaskId: 4, Value: 400},
		{Client: "node-b", TaskId: 3, Value: 500},
	}

	filtered := filterPingRecordsByScopedTasks(recs, scoped, "")
	if len(filtered) != 3 {
		t.Fatalf("filtered records = %d, want 3", len(filtered))
	}
	got := map[int]bool{}
	for _, rec := range filtered {
		got[rec.Value] = true
	}
	for _, want := range []int{100, 400, 500} {
		if !got[want] {
			t.Fatalf("filtered records missing value %d: %#v", want, filtered)
		}
	}
}

func TestScopedPingTasksForSpecificNodeExcludesDisabledAndUnscoped(t *testing.T) {
	pingTasks := []models.PingTask{
		{Id: 1, Enabled: true, DefaultOn: true},
		{Id: 2, Enabled: false, DefaultOn: true},
		{Id: 3, Enabled: true, Clients: models.StringArray{"node-b"}},
		{Id: 4, Enabled: true, Clients: models.StringArray{"node-a"}},
	}

	scoped := scopedPingTasksForRecords(pingTasks, -1, "node-a", nil)
	if len(scoped) != 2 {
		t.Fatalf("scoped tasks = %d, want 2", len(scoped))
	}
	if got, want := pingStatsCacheKey("node-a", scoped), "pingstats:node-a:1,4"; got != want {
		t.Fatalf("scoped task ids = %q, want %q", got, want)
	}
}

func TestScopedPingTasksFollowNodeOrder(t *testing.T) {
	pingTasks := []models.PingTask{
		{Id: 1, Enabled: true, Clients: models.StringArray{"node-a"}},
		{Id: 2, Enabled: true, Clients: models.StringArray{"node-a"}},
		{Id: 3, Enabled: true, Clients: models.StringArray{"node-a"}},
		{Id: 4, Enabled: false, Clients: models.StringArray{"node-a"}},
	}
	scoped := scopedPingTasksForRecords(pingTasks, -1, "node-a", models.UintArray{3, 4, 1, 2})
	if len(scoped) != 3 {
		t.Fatalf("scoped tasks = %d, want 3", len(scoped))
	}
	for index, taskID := range []uint{3, 1, 2} {
		if scoped[index].Id != taskID {
			t.Fatalf("task at %d = %d, want %d", index, scoped[index].Id, taskID)
		}
	}
}
