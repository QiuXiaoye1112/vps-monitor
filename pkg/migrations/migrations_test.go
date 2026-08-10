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

func TestBackfillTrafficCarrySeparatesAutomaticPositiveValues(t *testing.T) {
	db := openTestDB(t, "migrations_traffic_carry")
	if err := db.AutoMigrate(&appconfig.ConfigItem{}, &models.Client{}); err != nil {
		t.Fatalf("migrate traffic carry models: %v", err)
	}
	now := models.FromTime(time.Now())
	clients := []models.Client{
		{UUID: "positive", Token: "token-positive", TrafficComp: 1235, CreatedAt: now, UpdatedAt: now},
		{UUID: "negative", Token: "token-negative", TrafficComp: -55, CreatedAt: now, UpdatedAt: now},
		{
			UUID: "aggregate", Token: "token-aggregate",
			TrafficCarry: 5, TrafficCarryUp: 10, TrafficCarryDown: 20,
			CreatedAt: now, UpdatedAt: now,
		},
	}
	if err := db.Create(&clients).Error; err != nil {
		t.Fatalf("seed clients: %v", err)
	}

	if err := BackfillTrafficCarry(db); err != nil {
		t.Fatalf("backfill traffic carry: %v", err)
	}
	if err := BackfillTrafficCarry(db); err != nil {
		t.Fatalf("second traffic carry migration must be idempotent: %v", err)
	}

	var positive models.Client
	if err := db.First(&positive, "uuid = ?", "positive").Error; err != nil {
		t.Fatalf("load positive client: %v", err)
	}
	if positive.TrafficComp != 0 ||
		positive.TrafficCarry != 0 ||
		positive.TrafficCarryUp != 618 ||
		positive.TrafficCarryDown != 617 {
		t.Fatalf(
			"positive legacy value = comp %d aggregate %d up %d down %d, want 0/0/618/617",
			positive.TrafficComp,
			positive.TrafficCarry,
			positive.TrafficCarryUp,
			positive.TrafficCarryDown,
		)
	}

	var negative models.Client
	if err := db.First(&negative, "uuid = ?", "negative").Error; err != nil {
		t.Fatalf("load negative client: %v", err)
	}
	if negative.TrafficComp != -55 ||
		negative.TrafficCarry != 0 ||
		negative.TrafficCarryUp != 0 ||
		negative.TrafficCarryDown != 0 {
		t.Fatalf(
			"negative legacy value = comp %d aggregate %d up %d down %d, want -55/0/0/0",
			negative.TrafficComp,
			negative.TrafficCarry,
			negative.TrafficCarryUp,
			negative.TrafficCarryDown,
		)
	}

	var aggregate models.Client
	if err := db.First(&aggregate, "uuid = ?", "aggregate").Error; err != nil {
		t.Fatalf("load aggregate client: %v", err)
	}
	if aggregate.TrafficCarry != 0 ||
		aggregate.TrafficCarryUp != 13 ||
		aggregate.TrafficCarryDown != 22 {
		t.Fatalf(
			"aggregate legacy carry = aggregate %d up %d down %d, want 0/13/22",
			aggregate.TrafficCarry,
			aggregate.TrafficCarryUp,
			aggregate.TrafficCarryDown,
		)
	}
}

func TestBackfillDirectionalTrafficCarryFromVersionTwo(t *testing.T) {
	db := openTestDB(t, "migrations_directional_traffic_carry")
	if err := db.AutoMigrate(&appconfig.ConfigItem{}, &models.Client{}); err != nil {
		t.Fatalf("migrate directional traffic carry models: %v", err)
	}
	now := models.FromTime(time.Now())
	client := models.Client{
		UUID: "v2-client", Token: "token-v2-client",
		TrafficCarry: 9, TrafficCarryUp: 100, TrafficCarryDown: 200,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("seed v2 client: %v", err)
	}
	if err := db.Create(&appconfig.ConfigItem{
		Key: trafficCarryMigrationKey, Value: "true",
	}).Error; err != nil {
		t.Fatalf("seed v2 migration marker: %v", err)
	}

	if err := BackfillTrafficCarry(db); err != nil {
		t.Fatalf("backfill directional traffic carry: %v", err)
	}
	if err := BackfillTrafficCarry(db); err != nil {
		t.Fatalf("second directional migration must be idempotent: %v", err)
	}

	var updated models.Client
	if err := db.First(&updated, "uuid = ?", client.UUID).Error; err != nil {
		t.Fatalf("load migrated v2 client: %v", err)
	}
	if updated.TrafficCarry != 0 ||
		updated.TrafficCarryUp != 105 ||
		updated.TrafficCarryDown != 204 {
		t.Fatalf(
			"migrated v2 carry = aggregate %d up %d down %d, want 0/105/204",
			updated.TrafficCarry,
			updated.TrafficCarryUp,
			updated.TrafficCarryDown,
		)
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

func TestBackfillClientLastReportAtUsesNewestRetainedRecord(t *testing.T) {
	db := openTestDB(t, "migrations_last_report_at")
	if err := db.AutoMigrate(&models.Client{}, &models.Record{}); err != nil {
		t.Fatalf("migrate client and records: %v", err)
	}
	if err := db.Table("records_long_term").AutoMigrate(&models.Record{}); err != nil {
		t.Fatalf("migrate long-term records: %v", err)
	}

	client := models.Client{UUID: "client-last-report", Token: "token-last-report"}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create client: %v", err)
	}
	earlier := models.FromTime(time.Date(2026, time.August, 10, 5, 0, 0, 0, time.UTC))
	later := models.FromTime(time.Date(2026, time.August, 10, 6, 54, 49, 0, time.UTC))
	if err := db.Create(&models.Record{Client: client.UUID, Time: earlier}).Error; err != nil {
		t.Fatalf("create recent record: %v", err)
	}
	if err := db.Table("records_long_term").Create(&models.Record{Client: client.UUID, Time: later}).Error; err != nil {
		t.Fatalf("create long-term record: %v", err)
	}

	if err := BackfillClientLastReportAt(db); err != nil {
		t.Fatalf("backfill last report time: %v", err)
	}

	var reloaded models.Client
	if err := db.First(&reloaded, "uuid = ?", client.UUID).Error; err != nil {
		t.Fatalf("reload client: %v", err)
	}
	if got, want := reloaded.LastReportAt.ToTime(), later.ToTime(); !got.Equal(want) {
		t.Fatalf("last report time = %s, want %s", got, want)
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
