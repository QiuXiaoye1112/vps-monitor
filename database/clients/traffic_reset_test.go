package clients

import (
	"testing"
	"time"

	"github.com/monitor-monitor/monitor/database/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newClientTrafficTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Client{}))
	return db
}

func TestSaveClientDoesNotRefreshUnchangedCompensation(t *testing.T) {
	db := newClientTrafficTestDB(t)
	originalReset := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	originalUpdate := originalReset.Add(time.Hour)
	client := models.Client{
		UUID:               "unchanged-comp",
		Token:              "token-unchanged",
		Name:               "before",
		TrafficComp:        500,
		TrafficCompResetAt: models.FromTime(originalReset),
		CreatedAt:          models.FromTime(originalReset),
		UpdatedAt:          models.FromTime(originalUpdate),
	}
	require.NoError(t, db.Create(&client).Error)

	now := time.Date(2026, 7, 1, 0, 0, 5, 0, time.UTC)
	require.NoError(t, saveClient(db, map[string]interface{}{
		"uuid":                 client.UUID,
		"name":                 "after",
		"traffic_compensation": float64(500),
	}, now))

	var updated models.Client
	require.NoError(t, db.First(&updated, "uuid = ?", client.UUID).Error)
	require.Equal(t, "after", updated.Name)
	require.Equal(t, int64(500), updated.TrafficComp)
	require.True(t, updated.TrafficCompResetAt.ToTime().Equal(originalReset))
	require.True(t, updated.UpdatedAt.ToTime().Equal(now))
}

func TestSaveClientRefreshesChangedCompensation(t *testing.T) {
	db := newClientTrafficTestDB(t)
	originalReset := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	client := models.Client{
		UUID:               "changed-comp",
		Token:              "token-changed",
		TrafficComp:        500,
		TrafficCompResetAt: models.FromTime(originalReset),
		CreatedAt:          models.FromTime(originalReset),
		UpdatedAt:          models.FromTime(originalReset),
	}
	require.NoError(t, db.Create(&client).Error)

	now := time.Date(2026, 7, 1, 0, 0, 5, 0, time.UTC)
	require.NoError(t, saveClient(db, map[string]interface{}{
		"uuid":                 client.UUID,
		"traffic_compensation": float64(600),
	}, now))

	var updated models.Client
	require.NoError(t, db.First(&updated, "uuid = ?", client.UUID).Error)
	require.Equal(t, int64(600), updated.TrafficComp)
	require.True(t, updated.TrafficCompResetAt.ToTime().Equal(now))
}

func TestScheduledTrafficResetDoesNotOverwriteConcurrentEdit(t *testing.T) {
	db := newClientTrafficTestDB(t)
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	oldTime := time.Date(2026, 6, 1, 0, 0, 0, 0, loc)
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, loc)
	client := models.Client{
		UUID:                "reset-race",
		Token:               "token-race",
		TrafficResetEnabled: true,
		TrafficResetDay:     1,
		TrafficComp:         500,
		TrafficCarryUp:      100,
		TrafficCarryDown:    200,
		TrafficCompResetAt:  models.FromTime(oldTime),
		CreatedAt:           models.FromTime(oldTime),
		UpdatedAt:           models.FromTime(oldTime),
	}
	require.NoError(t, db.Create(&client).Error)

	var staleSnapshot models.Client
	require.NoError(t, db.First(&staleSnapshot, "uuid = ?", client.UUID).Error)

	concurrentUpdate := now.Add(time.Second)
	require.NoError(t, db.Model(&models.Client{}).Where("uuid = ?", client.UUID).Updates(map[string]interface{}{
		"traffic_compensation":          int64(900),
		"traffic_compensation_reset_at": concurrentUpdate,
		"updated_at":                    concurrentUpdate,
	}).Error)

	reset, err := resetTrafficCompensationIfDue(db, staleSnapshot, now)
	require.NoError(t, err)
	require.False(t, reset)

	var updated models.Client
	require.NoError(t, db.First(&updated, "uuid = ?", client.UUID).Error)
	require.Equal(t, int64(900), updated.TrafficComp)
	require.Equal(t, "2026-07-15 12:00:01", updated.TrafficCompResetAt.ToTime().Format("2006-01-02 15:04:05"))
}

func TestScheduledTrafficResetPersistsExactWindowBoundary(t *testing.T) {
	db := newClientTrafficTestDB(t)
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	oldTime := time.Date(2026, 6, 1, 0, 0, 0, 0, loc)
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, loc)
	client := models.Client{
		UUID:                "due-reset",
		Token:               "token-due",
		TrafficResetEnabled: true,
		TrafficResetDay:     1,
		TrafficComp:         500,
		TrafficCarry:        50,
		TrafficCarryUp:      100,
		TrafficCarryDown:    200,
		TrafficCompResetAt:  models.FromTime(oldTime),
		CreatedAt:           models.FromTime(oldTime),
		UpdatedAt:           models.FromTime(oldTime),
	}
	require.NoError(t, db.Create(&client).Error)

	var snapshot models.Client
	require.NoError(t, db.First(&snapshot, "uuid = ?", client.UUID).Error)
	reset, err := resetTrafficCompensationIfDue(db, snapshot, now)
	require.NoError(t, err)
	require.True(t, reset)

	var updated models.Client
	require.NoError(t, db.First(&updated, "uuid = ?", client.UUID).Error)
	require.Zero(t, updated.TrafficComp)
	require.Zero(t, updated.TrafficCarry)
	require.Zero(t, updated.TrafficCarryUp)
	require.Zero(t, updated.TrafficCarryDown)
	require.Equal(t, "2026-07-01 00:00:00", updated.TrafficCompResetAt.ToTime().Format("2006-01-02 15:04:05"))
}
