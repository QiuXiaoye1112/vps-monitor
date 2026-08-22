package records

import (
	"testing"
	"time"

	"github.com/monitor-monitor/monitor/database/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newRecordsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Record{}))
	require.NoError(t, db.Table("records_long_term").AutoMigrate(&models.Record{}))
	return db
}

func TestCompactionKeepsLatestAgentCycleTotals(t *testing.T) {
	db := newRecordsTestDB(t)
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	slot := now.Add(-5 * time.Hour).Truncate(15 * time.Minute)
	for _, record := range []models.Record{
		{Client: "node", Time: models.FromTime(slot), NetTotalUp: 100, NetTotalDown: 200},
		{Client: "node", Time: models.FromTime(slot.Add(10 * time.Minute)), NetTotalUp: 150, NetTotalDown: 260},
	} {
		require.NoError(t, db.Create(&record).Error)
	}
	require.NoError(t, migrateOldRecordsAt(db, now))
	var compacted models.Record
	require.NoError(t, db.Table("records_long_term").Where("client = ?", "node").First(&compacted).Error)
	require.Equal(t, int64(150), compacted.NetTotalUp)
	require.Equal(t, int64(260), compacted.NetTotalDown)
}

func TestHistoryCleanupOnlyDeletesOldRows(t *testing.T) {
	db := newRecordsTestDB(t)
	cutoff := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	old := models.Record{Client: "node", Time: models.FromTime(cutoff.Add(-time.Minute))}
	current := models.Record{Client: "node", Time: models.FromTime(cutoff.Add(time.Minute))}
	require.NoError(t, db.Create(&old).Error)
	require.NoError(t, db.Create(&current).Error)
	require.NoError(t, db.Table("records_long_term").Create(&old).Error)
	require.NoError(t, db.Table("records_long_term").Create(&current).Error)

	require.NoError(t, deleteLegacyRecordsBefore(db, cutoff, cutoff))
	for _, table := range []string{"records", "records_long_term"} {
		var count int64
		require.NoError(t, db.Table(table).Count(&count).Error)
		require.Equal(t, int64(1), count)
	}
}
