package clients

import (
	"testing"
	"time"

	"github.com/monitor-monitor/monitor/database/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDeleteClientDataRemovesAllNodeDataAndTaskMemberships(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Client{},
		&models.Record{},
		&models.PingTask{},
		&models.PingRecord{},
		&models.Task{},
		&models.TaskResult{},
	))
	require.NoError(t, db.Table("records_long_term").AutoMigrate(&models.Record{}))
	require.NoError(t, db.Table("history_records").AutoMigrate(&models.Record{}))

	deleted := models.Client{UUID: "delete-me", Token: "delete-token"}
	kept := models.Client{UUID: "keep-me", Token: "keep-token"}
	require.NoError(t, db.Create(&deleted).Error)
	require.NoError(t, db.Create(&kept).Error)

	now := models.FromTime(time.Now())
	require.NoError(t, db.Create(&models.Record{Client: deleted.UUID, Time: now}).Error)
	require.NoError(t, db.Table("records_long_term").Create(&models.Record{Client: deleted.UUID, Time: now}).Error)
	require.NoError(t, db.Table("history_records").Create(&models.Record{Client: deleted.UUID, Time: now}).Error)

	pingTask := models.PingTask{Name: "shared-ping", Clients: models.StringArray{deleted.UUID, kept.UUID}, Target: "127.0.0.1", Interval: 60}
	require.NoError(t, db.Create(&pingTask).Error)
	require.NoError(t, db.Create(&models.PingRecord{Client: deleted.UUID, TaskId: pingTask.Id, Time: now, Value: 1}).Error)

	sharedTask := models.Task{TaskId: "shared", Clients: models.StringArray{deleted.UUID, kept.UUID}, Command: "true"}
	soloTask := models.Task{TaskId: "solo", Clients: models.StringArray{deleted.UUID}, Command: "true"}
	require.NoError(t, db.Create(&sharedTask).Error)
	require.NoError(t, db.Create(&soloTask).Error)
	require.NoError(t, db.Create(&[]models.TaskResult{
		{TaskId: sharedTask.TaskId, Client: deleted.UUID, CreatedAt: now},
		{TaskId: sharedTask.TaskId, Client: kept.UUID, CreatedAt: now},
		{TaskId: soloTask.TaskId, Client: deleted.UUID, CreatedAt: now},
	}).Error)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return deleteClientData(tx, deleted.UUID)
	}))

	for _, table := range []string{"records", "records_long_term", "history_records", "ping_records"} {
		var count int64
		require.NoError(t, db.Table(table).Where("client = ?", deleted.UUID).Count(&count).Error)
		require.Zero(t, count, table)
	}
	var clientCount int64
	require.NoError(t, db.Model(&models.Client{}).Where("uuid = ?", deleted.UUID).Count(&clientCount).Error)
	require.Zero(t, clientCount)
	var deletedResults int64
	require.NoError(t, db.Model(&models.TaskResult{}).Where("client = ?", deleted.UUID).Count(&deletedResults).Error)
	require.Zero(t, deletedResults)

	var updatedPing models.PingTask
	require.NoError(t, db.First(&updatedPing, pingTask.Id).Error)
	require.Equal(t, models.StringArray{kept.UUID}, updatedPing.Clients)

	var updatedShared models.Task
	require.NoError(t, db.Where("task_id = ?", sharedTask.TaskId).First(&updatedShared).Error)
	require.Equal(t, models.StringArray{kept.UUID}, updatedShared.Clients)
	var soloCount int64
	require.NoError(t, db.Model(&models.Task{}).Where("task_id = ?", soloTask.TaskId).Count(&soloCount).Error)
	require.Zero(t, soloCount)
}
