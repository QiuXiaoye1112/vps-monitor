package auditlog

import (
	"testing"
	"time"

	"github.com/monitor-monitor/monitor/database/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRemoveOldLogsUsesStoredLocalTimeFormat(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Log{}))

	now := time.Date(2026, time.August, 10, 3, 45, 0, 0, time.UTC)
	threshold := now.AddDate(0, 0, -30)
	oldTime := models.FromTime(threshold.Add(-time.Minute))
	keptTime := models.FromTime(threshold.Add(time.Minute))
	oldStored, err := oldTime.Value()
	require.NoError(t, err)
	keptStored, err := keptTime.Value()
	require.NoError(t, err)

	require.NoError(t, db.Exec(
		"INSERT INTO logs (ip, uuid, message, msg_type, time) VALUES (?, ?, ?, ?, ?), (?, ?, ?, ?, ?)",
		"127.0.0.1", "old", "old", "event", oldStored,
		"127.0.0.1", "kept", "kept", "event", keptStored,
	).Error)

	require.NoError(t, removeOldLogs(db, now))

	var remaining []models.Log
	require.NoError(t, db.Order("uuid ASC").Find(&remaining).Error)
	require.Len(t, remaining, 1)
	require.Equal(t, "kept", remaining[0].UUID)
}
