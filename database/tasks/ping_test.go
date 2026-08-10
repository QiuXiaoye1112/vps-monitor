package tasks

import (
	"reflect"
	"testing"
	"time"

	"github.com/monitor-monitor/monitor/database/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
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

func TestDeletePingTaskKeepsHistoricalRecords(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Client{}, &models.PingTask{}, &models.PingRecord{}))

	client := models.Client{UUID: "node", Token: "token", PingTaskOrder: models.UintArray{1}}
	require.NoError(t, db.Create(&client).Error)
	task := models.PingTask{Name: "ping", Clients: models.StringArray{client.UUID}, Target: "127.0.0.1", Interval: 60}
	require.NoError(t, db.Create(&task).Error)
	require.NoError(t, db.Model(&models.Client{}).Where("uuid = ?", client.UUID).Update("ping_task_order", models.UintArray{task.Id}).Error)
	require.NoError(t, db.Create(&models.PingRecord{
		Client: client.UUID, TaskId: task.Id, Time: models.FromTime(time.Now()), Value: 10,
	}).Error)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return deletePingTasksTx(tx, []uint{task.Id})
	}))

	var taskCount, recordCount int64
	require.NoError(t, db.Model(&models.PingTask{}).Where("id = ?", task.Id).Count(&taskCount).Error)
	require.NoError(t, db.Model(&models.PingRecord{}).Where("task_id = ?", task.Id).Count(&recordCount).Error)
	require.Zero(t, taskCount)
	require.Equal(t, int64(1), recordCount)
}

func TestGetPingRecordsUsesStoredTimeFormat(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Client{}, &models.PingTask{}, &models.PingRecord{}))

	appLoc := models.GetAppLocation()
	reset := time.Date(2026, 8, 10, 3, 45, 0, 0, time.UTC)
	end := time.Date(2026, 8, 10, 5, 7, 37, 0, time.UTC)
	storageTime := func(instant time.Time) string {
		return instant.In(appLoc).Format("2006-01-02 15:04:05.0000000")
	}
	for _, row := range []struct {
		stamp string
		value int
	}{
		{storageTime(reset.Add(-time.Minute)), 1},
		{storageTime(reset), 2},
		{storageTime(time.Date(2026, 8, 10, 5, 0, 0, 0, time.UTC)), 3},
	} {
		require.NoError(t, db.Exec("INSERT INTO ping_records (client, task_id, time, value) VALUES (?, ?, ?, ?)",
			"ping-node", 7, row.stamp, row.value).Error)
	}

	records, err := getPingRecords(db, "ping-node", -1, reset, end)
	require.NoError(t, err)
	require.Len(t, records, 2)
	require.Equal(t, 3, records[0].Value)
	require.Equal(t, 2, records[1].Value)
}
