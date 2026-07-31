package clients

import (
	"path/filepath"
	"testing"

	"github.com/monitor-monitor/monitor/cmd/flags"
	"github.com/monitor-monitor/monitor/database/dbcore"
	"github.com/monitor-monitor/monitor/database/models"
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
