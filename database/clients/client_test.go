package clients

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/monitor-monitor/monitor/cmd/flags"
	"github.com/monitor-monitor/monitor/database/dbcore"
	"github.com/monitor-monitor/monitor/database/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCreateClientWithNameAndGroupPersistsGroup(t *testing.T) {
	flags.DatabaseType = flags.DatabaseTypeSQLite
	flags.DatabaseFile = filepath.Join(t.TempDir(), "monitor.db")

	uuid, token, err := CreateClientWithNameAndGroup(" node-a ", " prod ")
	if err != nil {
		t.Fatalf("CreateClientWithNameAndGroup returned error: %v", err)
	}
	if uuid == "" {
		t.Fatal("expected uuid to be generated")
	}
	if token == "" {
		t.Fatal("expected token to be generated")
	}

	client, err := GetClientByUUID(uuid)
	if err != nil {
		t.Fatalf("GetClientByUUID returned error: %v", err)
	}
	if client.Name != "node-a" {
		t.Fatalf("expected trimmed name %q, got %q", "node-a", client.Name)
	}
	if client.Group != "prod" {
		t.Fatalf("expected trimmed group %q, got %q", "prod", client.Group)
	}
}

func TestCreateClientPrependsNewNodes(t *testing.T) {
	flags.DatabaseType = flags.DatabaseTypeSQLite
	flags.DatabaseFile = filepath.Join(t.TempDir(), "monitor.db")
	db := dbcore.GetDBInstance()

	for _, client := range []models.Client{
		{UUID: "existing-first", Token: "token-first", Name: "first", Weight: 0},
		{UUID: "existing-second", Token: "token-second", Name: "second", Weight: 1},
	} {
		if err := db.Create(&client).Error; err != nil {
			t.Fatalf("seed client: %v", err)
		}
	}

	newUUID, _, err := CreateClientWithName("newest")
	if err != nil {
		t.Fatalf("CreateClientWithName returned error: %v", err)
	}
	created, err := GetClientByUUID(newUUID)
	if err != nil {
		t.Fatalf("GetClientByUUID returned error: %v", err)
	}
	if created.Weight != -1 {
		t.Fatalf("new client weight = %d, want -1", created.Weight)
	}

	ordered, err := GetAllClientBasicInfo()
	if err != nil {
		t.Fatalf("GetAllClientBasicInfo returned error: %v", err)
	}
	if len(ordered) == 0 || ordered[0].UUID != newUUID {
		t.Fatalf("new client was not first: %+v", ordered)
	}
}

func TestSaveClientStoresLocalTimeUpdatesInConfiguredFormat(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Client{}))
	require.NoError(t, db.Create(&models.Client{
		UUID:                "node-1",
		Token:               "token-1",
		TrafficResetEnabled: true,
	}).Error)

	now := time.Date(2026, time.August, 10, 3, 45, 0, 0, time.UTC)
	require.NoError(t, saveClient(db, map[string]interface{}{
		"uuid":                          "node-1",
		"name":                          "updated",
		"traffic_compensation":          123,
		"traffic_compensation_base":     456,
		"traffic_compensation_reset_at": now,
	}, now))

	var updatedAt string
	require.NoError(t, db.Table("clients").Select("CAST(updated_at AS TEXT)").Where("uuid = ?", "node-1").Row().Scan(&updatedAt))
	expected, err := models.FromTime(now).Value()
	require.NoError(t, err)
	require.Equal(t, expected, updatedAt)
	var saved models.Client
	require.NoError(t, db.First(&saved, "uuid = ?", "node-1").Error)
	require.Equal(t, "updated", saved.Name)
}

func TestUpdateLastReportAtPersistsAcrossClientReload(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Client{}))

	updatedAt := models.FromTime(time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC))
	client := models.Client{UUID: "node-last-report", Token: "token-last-report", UpdatedAt: updatedAt}
	require.NoError(t, db.Create(&client).Error)

	reportedAt := time.Date(2026, time.August, 10, 6, 54, 49, 0, time.UTC)
	require.NoError(t, updateLastReportAt(db, client.UUID, reportedAt))

	var reloaded models.Client
	require.NoError(t, db.First(&reloaded, "uuid = ?", client.UUID).Error)
	require.True(t, reloaded.LastReportAt.ToTime().Equal(reportedAt))
	require.True(t, reloaded.UpdatedAt.ToTime().Equal(updatedAt.ToTime()))
}

func TestValidateTrafficResetTimezone(t *testing.T) {
	for _, value := range []string{"UTC", "UTC+08:00", "UTC-05:30", "Asia/Tokyo", "America/New_York"} {
		require.NoError(t, ValidateTrafficResetTimezone(value), value)
	}
	for _, value := range []string{"", "UTC+15:00", "UTC+08:99", "UTC+8:5", "Mars/Olympus"} {
		require.Error(t, ValidateTrafficResetTimezone(value), value)
	}
}

func TestSaveClientPersistsTrafficResetTimezone(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Client{}))
	require.NoError(t, db.Create(&models.Client{UUID: "node-timezone", Token: "token-timezone"}).Error)

	require.NoError(t, saveClient(db, map[string]interface{}{
		"uuid":                   "node-timezone",
		"traffic_reset_timezone": "UTC-05:00",
	}, time.Now()))
	var saved models.Client
	require.NoError(t, db.First(&saved, "uuid = ?", "node-timezone").Error)
	require.Equal(t, "UTC-05:00", saved.TrafficResetTimezone)
}
