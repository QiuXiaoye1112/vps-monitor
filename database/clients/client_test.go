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
		TrafficComp:         1,
		TrafficResetEnabled: true,
	}).Error)

	now := time.Date(2026, time.August, 10, 3, 45, 0, 0, time.UTC)
	require.NoError(t, saveClient(db, map[string]interface{}{
		"uuid":                 "node-1",
		"traffic_compensation": 2,
	}, now))

	var updatedAt, resetAt string
	require.NoError(t, db.Table("clients").Select("CAST(updated_at AS TEXT), CAST(traffic_compensation_reset_at AS TEXT)").Where("uuid = ?", "node-1").Row().Scan(&updatedAt, &resetAt))
	expected, err := models.FromTime(now).Value()
	require.NoError(t, err)
	require.Equal(t, expected, updatedAt)
	require.Equal(t, expected, resetAt)
}
