package accounts

import (
	"testing"
	"time"

	"github.com/monitor-monitor/monitor/database/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSessionTimeQueriesAndUpdatesUseStoredLocalTimeFormat(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Session{}))

	now := time.Date(2026, time.August, 10, 3, 45, 0, 0, time.UTC)
	expired := models.FromTime(now.Add(-time.Minute))
	active := models.FromTime(now.Add(time.Minute))
	expiredStored, err := expired.Value()
	require.NoError(t, err)
	activeStored, err := active.Value()
	require.NoError(t, err)

	require.NoError(t, db.Exec(
		"INSERT INTO sessions (uuid, session, user_agent, ip, login_method, latest_online, latest_user_agent, latest_ip, expires, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?), (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"user-1", "expired", "ua", "ip", "password", activeStored, "", "", expiredStored, activeStored,
		"user-1", "active", "ua", "ip", "password", activeStored, "", "", activeStored, activeStored,
	).Error)

	require.NoError(t, updateLatestOnline(db, "active", now.Add(2*time.Minute)))
	require.NoError(t, updateLatest(db, "active", "new-ua", "new-ip", now.Add(3*time.Minute)))

	var rawLatest string
	var latestUserAgent, latestIP string
	require.NoError(t, db.Table("sessions").Select("CAST(latest_online AS TEXT), latest_user_agent, latest_ip").Where("session = ?", "active").Row().Scan(&rawLatest, &latestUserAgent, &latestIP))
	expectedLatest, err := models.FromTime(now.Add(3 * time.Minute)).Value()
	require.NoError(t, err)
	require.Equal(t, expectedLatest, rawLatest)
	require.Equal(t, "new-ua", latestUserAgent)
	require.Equal(t, "new-ip", latestIP)

	result := removeExpiredSessions(db, now)
	require.NoError(t, result.Error)
	require.EqualValues(t, 1, result.RowsAffected)

	var count int64
	require.NoError(t, db.Model(&models.Session{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}
