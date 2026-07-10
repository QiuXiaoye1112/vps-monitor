package clients

import (
	"path/filepath"
	"testing"

	"github.com/monitor-monitor/monitor/cmd/flags"
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
