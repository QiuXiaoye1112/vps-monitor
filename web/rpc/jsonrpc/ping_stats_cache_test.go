package jsonrpc

import (
	"testing"

	"github.com/monitor-monitor/monitor/database/models"
)

func TestPingStatsCacheKeyUsesOnlyEnabledScopedTasks(t *testing.T) {
	const uuid = "node-a"
	pingTasks := []models.PingTask{
		{Id: 4, Enabled: true, DefaultOn: true},
		{Id: 2, Enabled: false, DefaultOn: true},
		{Id: 3, Enabled: true, Clients: models.StringArray{"node-b"}},
		{Id: 1, Enabled: true, Clients: models.StringArray{"node-a"}},
	}

	assigned := assignedPingTasksForNode(uuid, pingTasks)
	if len(assigned) != 2 {
		t.Fatalf("assigned tasks = %d, want 2", len(assigned))
	}

	if got, want := pingStatsCacheKey(uuid, assigned), "pingstats:node-a:1,4"; got != want {
		t.Fatalf("cache key = %q, want %q", got, want)
	}
}

func TestPingStatsCacheKeyChangesWhenTaskScopeChanges(t *testing.T) {
	const uuid = "node-a"
	before := assignedPingTasksForNode(uuid, []models.PingTask{
		{Id: 7, Enabled: true, Clients: models.StringArray{"node-a"}},
	})
	after := assignedPingTasksForNode(uuid, []models.PingTask{
		{Id: 7, Enabled: true, Clients: models.StringArray{"node-b"}},
	})

	beforeKey := pingStatsCacheKey(uuid, before)
	afterKey := pingStatsCacheKey(uuid, after)
	if beforeKey == afterKey {
		t.Fatalf("cache key did not change after task scope changed: %q", beforeKey)
	}
	if got, want := afterKey, "pingstats:node-a:"; got != want {
		t.Fatalf("cache key after removing scope = %q, want %q", got, want)
	}
}
