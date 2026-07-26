package records

import (
	"testing"
	"time"

	"github.com/monitor-monitor/monitor/database/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestHistoryRecordsKeepRawPrecisionAndAggregateLongRanges(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Table(HistoryTable).AutoMigrate(&models.Record{}))

	start := time.Date(2026, 7, 26, 12, 0, 0, 0, time.Local)
	for i := 0; i < 12; i++ {
		require.NoError(t, db.Table(HistoryTable).Create(&models.Record{
			Client:         "node",
			Time:           models.FromTime(start.Add(time.Duration(i) * 5 * time.Second)),
			Cpu:            float32(i),
			Ram:            int64(100 + i),
			RamTotal:       1000,
			Process:        20 + i,
			Connections:    30 + i,
			ConnectionsUdp: 2,
		}).Error)
	}

	raw, err := getHistoryRecordsByClientAndTime(db, "node", start, start.Add(time.Minute), 20)
	require.NoError(t, err)
	require.Len(t, raw, 12)
	require.Equal(t, 5*time.Second, raw[1].Time.ToTime().Sub(raw[0].Time.ToTime()))

	aggregated, err := getHistoryRecordsByClientAndTime(db, "node", start, start.Add(time.Minute), 4)
	require.NoError(t, err)
	require.LessOrEqual(t, len(aggregated), 4)
	require.NotEmpty(t, aggregated)
	require.Equal(t, int64(1000), aggregated[0].RamTotal)
	require.Greater(t, aggregated[len(aggregated)-1].Cpu, aggregated[0].Cpu)
}
