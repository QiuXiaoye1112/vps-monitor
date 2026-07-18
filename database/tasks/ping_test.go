package tasks

import (
	"reflect"
	"testing"

	"github.com/monitor-monitor/monitor/database/models"
)

func TestOrderPingTasksUsesClientSelectionAndOrder(t *testing.T) {
	candidates := []models.PingTask{
		{Id: 1, Name: "one"},
		{Id: 2, Name: "two"},
		{Id: 3, Name: "three"},
	}
	got := OrderPingTasks(models.UintArray{3, 1}, candidates)
	want := []models.PingTask{candidates[2], candidates[0]}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ordered tasks = %#v, want %#v", got, want)
	}
}
