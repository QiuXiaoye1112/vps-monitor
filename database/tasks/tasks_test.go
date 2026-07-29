package tasks

import (
	"testing"
	"time"

	"github.com/monitor-monitor/monitor/database/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type legacyTaskResult struct {
	TaskId     string            `gorm:"type:varchar(36);index"`
	Client     string            `gorm:"type:varchar(36)"`
	ClientInfo models.Client     `gorm:"foreignKey:Client;references:UUID;constraint:OnDelete:CASCADE,OnUpdate:CASCADE"`
	Result     string            `gorm:"type:longtext"`
	ExitCode   *int              `gorm:"type:int"`
	FinishedAt *models.LocalTime `gorm:"type:timestamp"`
	CreatedAt  models.LocalTime  `gorm:"type:timestamp"`
}

func (legacyTaskResult) TableName() string {
	return "task_results"
}

func TestSaveTaskResultIsIdempotentByClientAndTask(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.TaskResult{}))
	require.True(t, db.Migrator().HasIndex(&models.TaskResult{}, "idx_client_task"))

	firstAt := models.FromTime(time.Now().Add(-time.Minute))
	secondAt := models.FromTime(time.Now())
	require.NoError(t, saveTaskResult(db, "task-1", "node-1", "first", 1, firstAt))
	require.NoError(t, saveTaskResult(db, "task-1", "node-1", "second", 0, secondAt))

	var count int64
	require.NoError(t, db.Model(&models.TaskResult{}).
		Where("task_id = ? AND client = ?", "task-1", "node-1").
		Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.Error(t, db.Create(&models.TaskResult{
		TaskId:    "task-1",
		Client:    "node-1",
		CreatedAt: models.FromTime(time.Now()),
	}).Error)

	var result models.TaskResult
	require.NoError(t, db.Where("task_id = ? AND client = ?", "task-1", "node-1").First(&result).Error)
	require.Equal(t, "second", result.Result)
	require.NotNil(t, result.ExitCode)
	require.Equal(t, 0, *result.ExitCode)
	require.NotNil(t, result.FinishedAt)
	require.Equal(t, secondAt.ToTime().UnixNano(), result.FinishedAt.ToTime().UnixNano())

	require.NoError(t, saveTaskResult(db, "task-1", "node-2", "other node", 0, secondAt))
	require.NoError(t, saveTaskResult(db, "task-2", "node-1", "other task", 0, secondAt))
	require.NoError(t, db.Model(&models.TaskResult{}).Count(&count).Error)
	require.EqualValues(t, 3, count)
}

func TestTaskResultUniqueIndexMigratesExistingTable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Client{}))
	require.NoError(t, db.Create(&models.Client{UUID: "node-1", Token: "token-1"}).Error)
	require.NoError(t, db.AutoMigrate(&legacyTaskResult{}))
	require.NoError(t, db.Create(&legacyTaskResult{
		TaskId:    "task-1",
		Client:    "node-1",
		Result:    "existing",
		CreatedAt: models.FromTime(time.Now()),
	}).Error)

	require.NoError(t, db.AutoMigrate(&models.TaskResult{}))
	require.True(t, db.Migrator().HasIndex(&models.TaskResult{}, "idx_client_task"))
	var indexSQL string
	require.NoError(t, db.Raw(
		"SELECT sql FROM sqlite_master WHERE type = 'index' AND name = ?",
		"idx_client_task",
	).Scan(&indexSQL).Error)
	require.Contains(t, indexSQL, "UNIQUE INDEX")
	var existingCount int64
	require.NoError(t, db.Model(&models.TaskResult{}).
		Where("task_id = ? AND client = ?", "task-1", "node-1").
		Count(&existingCount).Error)
	require.EqualValues(t, 1, existingCount)
	require.Error(t, db.Create(&models.TaskResult{
		TaskId:    "task-1",
		Client:    "node-1",
		CreatedAt: models.FromTime(time.Now()),
	}).Error)
}
