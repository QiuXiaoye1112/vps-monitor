package migrations

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/monitor-monitor/monitor/database/models"
	appconfig "github.com/monitor-monitor/monitor/pkg/config"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()

	dsn := "file:" + strings.ReplaceAll(name, " ", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite test db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sqlite test db handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	return db
}

func TestHasLegacyConfigTable(t *testing.T) {
	t.Run("config item table", func(t *testing.T) {
		db := openTestDB(t, "migrations_config_item")
		if err := db.AutoMigrate(&appconfig.ConfigItem{}); err != nil {
			t.Fatalf("migrate config item table: %v", err)
		}
		if hasLegacyConfigTable(db) {
			t.Fatal("config item table was detected as legacy config table")
		}
	})

	t.Run("legacy config table", func(t *testing.T) {
		db := openTestDB(t, "migrations_legacy_config")
		if err := db.AutoMigrate(&legacyModelConfig{}); err != nil {
			t.Fatalf("migrate legacy config table: %v", err)
		}
		if !hasLegacyConfigTable(db) {
			t.Fatal("legacy config table was not detected")
		}
	})
}

func TestRunSkipsLegacyConfigMigrationForCurrentConfigItemTable(t *testing.T) {
	db := openTestDB(t, "migrations_config_item_run")
	if err := db.AutoMigrate(&appconfig.ConfigItem{}); err != nil {
		t.Fatalf("migrate config item table: %v", err)
	}
	if err := db.Create(&appconfig.ConfigItem{Key: "o_auth_provider", Value: `"github"`}).Error; err != nil {
		t.Fatalf("seed config item: %v", err)
	}

	if err := Run(Context{DB: db}); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	if db.Migrator().HasColumn(&legacyModelConfig{}, "id") {
		t.Fatal("config item table was changed into the legacy config shape")
	}
	if db.Migrator().HasTable(&models.OidcProvider{}) {
		t.Fatal("legacy OIDC migration ran against the config item table")
	}

	var item appconfig.ConfigItem
	if err := db.First(&item, "key = ?", "o_auth_provider").Error; err != nil {
		t.Fatalf("config item was not preserved: %v", err)
	}
	if item.Value != `"github"` {
		t.Fatalf("unexpected config value: %s", item.Value)
	}
}

func TestRunPreservesVersion120RuntimeShape(t *testing.T) {
	db := openTestDB(t, "migrations_v120_runtime_shape")
	if err := db.AutoMigrate(
		&appconfig.ConfigItem{},
		&models.OidcProvider{},
		&models.Client{},
		&models.PingTask{},
	); err != nil {
		t.Fatalf("migrate 1.2.0 runtime shape: %v", err)
	}
	now := models.FromTime(time.Now())
	if err := db.Create(&models.Client{UUID: "client-a", Token: "token-a", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("seed client: %v", err)
	}
	if err := db.Create(&models.PingTask{
		Name:      "already explicit",
		Clients:   models.StringArray{"client-a"},
		DefaultOn: true,
		Type:      "icmp",
		Target:    "example.com",
		Interval:  60,
	}).Error; err != nil {
		t.Fatalf("seed ping task: %v", err)
	}
	if err := db.Create(&appconfig.ConfigItem{Key: appconfig.OAuthProviderKey, Value: `"github"`}).Error; err != nil {
		t.Fatalf("seed config item: %v", err)
	}
	if err := db.Create(&models.OidcProvider{Name: "github", Addition: `{"client_id":"old","client_secret":"secret"}`}).Error; err != nil {
		t.Fatalf("seed oidc provider: %v", err)
	}

	if err := Run(Context{DB: db}); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	if db.Migrator().HasColumn(&legacyModelConfig{}, "sitename") {
		t.Fatal("current config item table was treated as legacy wide config")
	}

	var configItem appconfig.ConfigItem
	if err := db.First(&configItem, "key = ?", appconfig.OAuthProviderKey).Error; err != nil {
		t.Fatalf("find config item: %v", err)
	}
	if configItem.Value != `"github"` {
		t.Fatalf("unexpected config item value: %s", configItem.Value)
	}

	var oidc models.OidcProvider
	if err := db.First(&oidc, "name = ?", "github").Error; err != nil {
		t.Fatalf("find oidc provider: %v", err)
	}
	if oidc.Addition != `{"client_id":"old","client_secret":"secret"}` {
		t.Fatalf("oidc provider was unexpectedly changed: %s", oidc.Addition)
	}

	var task models.PingTask
	if err := db.First(&task, "name = ?", "already explicit").Error; err != nil {
		t.Fatalf("find ping task: %v", err)
	}
	if len(task.Clients) != 1 || task.Clients[0] != "client-a" {
		t.Fatalf("explicit ping task clients were changed: %v", task.Clients)
	}
}

func TestRunMigratesLegacyConfigTableToConfigItems(t *testing.T) {
	db := openTestDB(t, "migrations_legacy_config_to_items")
	if err := db.AutoMigrate(&legacyModelConfig{}); err != nil {
		t.Fatalf("migrate legacy config table: %v", err)
	}
	legacy := legacyModelConfig{
		Sitename:               "Old Monitor",
		Description:            "legacy description",
		Theme:                  "classic",
		GeoIpEnabled:           true,
		GeoIpProvider:          "ip-api",
		OAuthProvider:          "github",
		RecordEnabled:          true,
		RecordPreserveTime:     48,
		PingRecordPreserveTime: 12,
	}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatalf("seed legacy config: %v", err)
	}

	if err := Run(Context{DB: db}); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	if db.Migrator().HasColumn(&legacyModelConfig{}, "sitename") {
		t.Fatal("legacy config columns were not removed")
	}

	var sitename appconfig.ConfigItem
	if err := db.First(&sitename, "key = ?", appconfig.SitenameKey).Error; err != nil {
		t.Fatalf("find migrated sitename: %v", err)
	}
	if sitename.Value != `"Old Monitor"` {
		t.Fatalf("unexpected sitename value: %s", sitename.Value)
	}

	var corsOriginCheck appconfig.ConfigItem
	if err := db.First(&corsOriginCheck, "key = ?", appconfig.CorsOriginCheckEnabledKey).Error; err == nil {
		t.Fatalf("unexpected migrated cors_origin_check_enabled value: %s", corsOriginCheck.Value)
	}
}

func TestRunExpandsLegacyPingAllClientsTasks(t *testing.T) {
	db := openTestDB(t, "migrations_ping_all_clients")
	if err := db.AutoMigrate(&models.Client{}); err != nil {
		t.Fatalf("migrate clients: %v", err)
	}
	if err := db.Exec(`
		CREATE TABLE ping_tasks (
			id integer primary key autoincrement,
			weight integer not null default 0,
			name varchar(255) not null,
			all_clients boolean not null default false,
			type varchar(12) not null default 'icmp',
			target varchar(255) not null,
			interval integer not null default 60
		)
	`).Error; err != nil {
		t.Fatalf("create legacy ping_tasks: %v", err)
	}
	now := models.FromTime(time.Now())
	clients := []models.Client{
		{UUID: "client-a", Token: "token-a", CreatedAt: now, UpdatedAt: now},
		{UUID: "client-b", Token: "token-b", CreatedAt: now, UpdatedAt: now},
	}
	if err := db.Create(&clients).Error; err != nil {
		t.Fatalf("seed clients: %v", err)
	}
	if err := db.Exec("INSERT INTO ping_tasks (name, all_clients, type, target, interval) VALUES (?, ?, ?, ?, ?)", "legacy task", true, "icmp", "example.com", 60).Error; err != nil {
		t.Fatalf("seed legacy ping task: %v", err)
	}

	if err := Run(Context{DB: db}); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	var task models.PingTask
	if err := db.First(&task).Error; err != nil {
		t.Fatalf("find migrated ping task: %v", err)
	}
	if len(task.Clients) != 2 {
		t.Fatalf("expected two migrated clients, got %v", task.Clients)
	}
	got := map[string]bool{}
	for _, uuid := range task.Clients {
		got[uuid] = true
	}
	if !got["client-a"] || !got["client-b"] {
		t.Fatalf("unexpected migrated clients: %v", task.Clients)
	}

	raw, err := json.Marshal(task.Clients)
	if err != nil {
		t.Fatalf("marshal migrated clients: %v", err)
	}
	if string(raw) != `["client-a","client-b"]` {
		t.Fatalf("unexpected clients json: %s", raw)
	}
}

func TestBackfillClientPingTaskOrderPreservesLegacyAssignments(t *testing.T) {
	db := openTestDB(t, "client_ping_task_order")
	if err := db.Exec("CREATE TABLE clients (uuid varchar(36) PRIMARY KEY, token varchar(255) NOT NULL UNIQUE, name varchar(100), weight integer, created_at timestamp, updated_at timestamp)").Error; err != nil {
		t.Fatalf("create legacy clients table: %v", err)
	}
	if err := db.AutoMigrate(&models.PingTask{}); err != nil {
		t.Fatalf("migrate ping tasks: %v", err)
	}
	if err := db.Exec("INSERT INTO clients (uuid, token, name, weight) VALUES (?, ?, ?, ?), (?, ?, ?, ?)",
		"node-a", "token-a", "A", 0, "node-b", "token-b", "B", 1).Error; err != nil {
		t.Fatalf("seed clients: %v", err)
	}
	pingTasks := []models.PingTask{
		{Weight: 20, Name: "default", DefaultOn: true, Enabled: true, Type: "tcp", Target: "a.example:80", Interval: 60},
		{Weight: 10, Name: "selected", Clients: models.StringArray{"node-a"}, Enabled: false, Type: "tcp", Target: "b.example:80", Interval: 60},
	}
	if err := db.Create(&pingTasks).Error; err != nil {
		t.Fatalf("seed ping tasks: %v", err)
	}
	if err := db.AutoMigrate(&models.Client{}); err != nil {
		t.Fatalf("add ping task order column: %v", err)
	}
	if err := BackfillClientPingTaskOrder(db); err != nil {
		t.Fatalf("backfill client ping task order: %v", err)
	}

	var clients []models.Client
	if err := db.Order("uuid ASC").Find(&clients).Error; err != nil {
		t.Fatalf("read migrated clients: %v", err)
	}
	wantNodeA := models.UintArray{pingTasks[1].Id, pingTasks[0].Id}
	if got, want := clients[0].PingTaskOrder, wantNodeA; !reflect.DeepEqual(got, want) {
		t.Fatalf("node-a order = %v, want %v", got, want)
	}
	wantNodeB := models.UintArray{pingTasks[0].Id}
	if got, want := clients[1].PingTaskOrder, wantNodeB; !reflect.DeepEqual(got, want) {
		t.Fatalf("node-b order = %v, want %v", got, want)
	}

	var migratedTasks []models.PingTask
	if err := db.Order("weight ASC").Find(&migratedTasks).Error; err != nil {
		t.Fatalf("read migrated tasks: %v", err)
	}
	if migratedTasks[0].DefaultOn || migratedTasks[1].DefaultOn {
		t.Fatal("legacy all_clients flags were not disabled")
	}
	if !migratedTasks[0].Enabled || !migratedTasks[1].Enabled {
		t.Fatal("legacy global disabled state was not removed")
	}
	wantSelectedClients := models.StringArray{"node-a"}
	if got, want := migratedTasks[0].Clients, wantSelectedClients; !reflect.DeepEqual(got, want) {
		t.Fatalf("selected task clients = %v, want %v", got, want)
	}
	wantDefaultClients := models.StringArray{"node-a", "node-b"}
	if got, want := migratedTasks[1].Clients, wantDefaultClients; !reflect.DeepEqual(got, want) {
		t.Fatalf("default task clients = %v, want %v", got, want)
	}
}
