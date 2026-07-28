package records

import (
	"testing"
	"time"

	"github.com/monitor-monitor/monitor/database/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTrafficTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Record{}))
	require.NoError(t, db.Table("records_long_term").AutoMigrate(&models.Record{}))
	return db
}

func TestSumLegacyTrafficDeltasJoinsAfterLatestArchivedSlot(t *testing.T) {
	db := newTrafficTestDB(t)
	client := "traffic-node"
	start := time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 18, 11, 0, 0, 0, time.UTC)

	// The 10:00 long-term row represents the complete [10:00, 10:15) slot.
	require.NoError(t, db.Table("records_long_term").Create(&models.Record{
		Client: client, Time: models.FromTime(time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)),
		TrafficUp: 100, TrafficDown: 200,
	}).Error)

	// Raw rows from an archived slot are intentionally retained for an hour by
	// compaction and must not be counted twice.
	require.NoError(t, db.Create(&models.Record{
		Client: client, Time: models.FromTime(time.Date(2026, 7, 18, 10, 5, 0, 0, time.UTC)),
		TrafficUp: 10, TrafficDown: 20,
	}).Error)
	// Rows immediately after the archived slot must be included even when the
	// scheduled compactor has not processed their slot yet.
	require.NoError(t, db.Create(&models.Record{
		Client: client, Time: models.FromTime(time.Date(2026, 7, 18, 10, 15, 0, 0, time.UTC)),
		TrafficUp: 30, TrafficDown: 40,
	}).Error)
	require.NoError(t, db.Create(&models.Record{
		Client: client, Time: models.FromTime(time.Date(2026, 7, 18, 10, 29, 0, 0, time.UTC)),
		TrafficUp: 50, TrafficDown: 60,
	}).Error)

	up, down, err := sumLegacyTrafficDeltas(db, client, start, end)
	require.NoError(t, err)
	require.Equal(t, int64(180), up)
	require.Equal(t, int64(300), down)
}

func TestSumLegacyTrafficDeltasUsesAllRawRowsWithoutArchive(t *testing.T) {
	db := newTrafficTestDB(t)
	client := "new-node"
	start := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	for i, value := range []int64{10, 20, 30} {
		require.NoError(t, db.Create(&models.Record{
			Client:    client,
			Time:      models.FromTime(start.Add(time.Duration(i) * time.Minute)),
			TrafficUp: value, TrafficDown: value * 2,
		}).Error)
	}

	up, down, err := sumLegacyTrafficDeltas(db, client, start, end)
	require.NoError(t, err)
	require.Equal(t, int64(60), up)
	require.Equal(t, int64(120), down)
}

func TestTrafficAccountingStartUsesLatestManualClear(t *testing.T) {
	windowStart := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	clearedAt := windowStart.Add(10 * 24 * time.Hour)

	client := models.Client{TrafficClearedAt: models.FromTime(clearedAt)}
	require.Equal(t, clearedAt, trafficAccountingStart(client, windowStart))

	nextWindow := windowStart.AddDate(0, 1, 0)
	require.Equal(t, nextWindow, trafficAccountingStart(client, nextWindow))
}

func TestTrafficWindowUsesConfiguredMinute(t *testing.T) {
	loc := trafficLocation()
	client := models.Client{
		TrafficResetEnabled: true,
		TrafficResetDay:     31,
		TrafficResetHour:    9,
		TrafficResetMinute:  45,
	}
	now := time.Date(2026, 4, 30, 9, 44, 0, 0, loc)

	start, end := TrafficWindow(client, now)
	require.Equal(t, time.Date(2026, 3, 31, 9, 45, 0, 0, loc), start)
	require.Equal(t, time.Date(2026, 4, 30, 9, 45, 0, 0, loc), end)

	start, end = TrafficWindow(client, end)
	require.Equal(t, time.Date(2026, 4, 30, 9, 45, 0, 0, loc), start)
	require.Equal(t, time.Date(2026, 5, 31, 9, 45, 0, 0, loc), end)
}

func TestTrafficAdjustmentsAreExcludedAtExactResetBoundary(t *testing.T) {
	loc := trafficLocation()
	client := models.Client{
		TrafficResetEnabled: true,
		TrafficResetDay:     31,
		TrafficResetHour:    9,
		TrafficResetMinute:  45,
		TrafficComp:         500,
		TrafficCarryUp:      100,
		TrafficCarryDown:    200,
		TrafficCompResetAt:  models.FromTime(time.Date(2026, 3, 31, 9, 45, 0, 0, loc)),
		CreatedAt:           models.FromTime(time.Date(2026, 3, 1, 0, 0, 0, 0, loc)),
	}
	boundary := time.Date(2026, 4, 30, 9, 45, 0, 0, loc)
	windowStart, _ := TrafficWindow(client, boundary)
	compensation, carryUp, carryDown := trafficAdjustmentsForWindow(client, windowStart)
	require.Zero(t, compensation)
	require.Zero(t, carryUp)
	require.Zero(t, carryDown)
}

func TestTrafficAdjustmentsChangedAfterBoundaryRemainCurrent(t *testing.T) {
	loc := trafficLocation()
	boundary := time.Date(2026, 4, 30, 9, 45, 0, 0, loc)
	client := models.Client{
		TrafficResetEnabled: true,
		TrafficResetDay:     31,
		TrafficResetHour:    9,
		TrafficResetMinute:  45,
		TrafficComp:         500,
		TrafficCarryUp:      100,
		TrafficCarryDown:    200,
		TrafficCompResetAt:  models.FromTime(boundary.Add(time.Minute)),
	}
	windowStart, _ := TrafficWindow(client, boundary.Add(2*time.Minute))
	compensation, carryUp, carryDown := trafficAdjustmentsForWindow(client, windowStart)
	require.Equal(t, int64(500), compensation)
	require.Equal(t, int64(100), carryUp)
	require.Equal(t, int64(200), carryDown)
}

func TestDeleteLegacyRecordsBeforeFoldsCurrentBillingWindow(t *testing.T) {
	db := newTrafficTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.Client{}))
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	historyCutoff := now.Add(-30 * 24 * time.Hour)
	client := models.Client{
		UUID: "long-month", TrafficResetEnabled: true,
		TrafficResetDay: 21, TrafficResetHour: 0,
	}
	require.NoError(t, db.Create(&client).Error)

	// The current Shanghai billing period started on July 21. The old history
	// row is deleted, but its traffic is folded into the hidden carry.
	recordTime := time.Date(2026, 7, 21, 5, 0, 0, 0, time.UTC)
	require.True(t, recordTime.Before(historyCutoff))
	require.NoError(t, db.Table("records_long_term").Create(&models.Record{
		Client: client.UUID, Time: models.FromTime(recordTime),
		TrafficUp: 123, TrafficDown: 456,
	}).Error)

	require.NoError(t, deleteLegacyRecordsBefore(db, historyCutoff, now))
	var count int64
	require.NoError(t, db.Table("records_long_term").Where("client = ?", client.UUID).Count(&count).Error)
	require.Zero(t, count)
	var updated models.Client
	require.NoError(t, db.Where("uuid = ?", client.UUID).First(&updated).Error)
	require.Equal(t, int64(0), updated.TrafficComp)
	require.Equal(t, int64(0), updated.TrafficCarry)
	require.Equal(t, int64(123), updated.TrafficCarryUp)
	require.Equal(t, int64(456), updated.TrafficCarryDown)
}

func TestDeleteLegacyRecordsBeforeDoesNotRestoreManuallyClearedTraffic(t *testing.T) {
	db := newTrafficTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.Client{}))
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	historyCutoff := now.Add(-30 * 24 * time.Hour)
	recordTime := time.Date(2026, 7, 21, 5, 0, 0, 0, time.UTC)
	client := models.Client{
		UUID:                "cleared-node",
		TrafficResetEnabled: true,
		TrafficResetDay:     21,
		TrafficClearedAt:    models.FromTime(recordTime.Add(24 * time.Hour)),
	}
	require.NoError(t, db.Create(&client).Error)
	require.NoError(t, db.Table("records_long_term").Create(&models.Record{
		Client: client.UUID, Time: models.FromTime(recordTime),
		TrafficUp: 123, TrafficDown: 456,
	}).Error)

	require.NoError(t, deleteLegacyRecordsBefore(db, historyCutoff, now))
	var updated models.Client
	require.NoError(t, db.Where("uuid = ?", client.UUID).First(&updated).Error)
	require.Zero(t, updated.TrafficCarryUp)
	require.Zero(t, updated.TrafficCarryDown)
}

func TestDeleteLegacyRecordsBeforeFoldsCumulativeLedgerWhenResetDisabled(t *testing.T) {
	db := newTrafficTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.Client{}))
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	client := models.Client{UUID: "no-reset", TrafficResetEnabled: false}
	require.NoError(t, db.Create(&client).Error)
	require.NoError(t, db.Model(&models.Client{}).Where("uuid = ?", client.UUID).
		Update("traffic_reset_enabled", false).Error)
	require.NoError(t, db.Table("records_long_term").Create(&models.Record{
		Client: client.UUID, Time: models.FromTime(now.AddDate(-1, 0, 0)), TrafficDown: 456,
	}).Error)

	require.NoError(t, deleteLegacyRecordsBefore(db, now.Add(-30*24*time.Hour), now))
	var count int64
	require.NoError(t, db.Table("records_long_term").Where("client = ?", client.UUID).Count(&count).Error)
	require.Zero(t, count)
	var updated models.Client
	require.NoError(t, db.Where("uuid = ?", client.UUID).First(&updated).Error)
	require.Equal(t, int64(0), updated.TrafficComp)
	require.Equal(t, int64(0), updated.TrafficCarry)
	require.Equal(t, int64(0), updated.TrafficCarryUp)
	require.Equal(t, int64(456), updated.TrafficCarryDown)
}
