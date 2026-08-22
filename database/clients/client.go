package clients

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/monitor-monitor/monitor/database/dbcore"
	"github.com/monitor-monitor/monitor/database/models"
	"github.com/monitor-monitor/monitor/database/tasks"
	"github.com/monitor-monitor/monitor/utils"
	"gorm.io/gorm"

	"github.com/google/uuid"
)

var clientCreationMu sync.Mutex

func DeleteClient(clientUuid string) error {
	if strings.TrimSpace(clientUuid) == "" {
		return fmt.Errorf("invalid client UUID")
	}

	db := dbcore.GetDBInstance()
	err := db.Transaction(func(tx *gorm.DB) error {
		return deleteClientData(tx, clientUuid)
	})
	if err != nil {
		return err
	}
	if err := tasks.ReloadPingSchedule(); err != nil {
		log.Printf("failed to reload ping schedule after deleting client %s: %v", clientUuid, err)
	}
	return nil
}

func deleteClientData(tx *gorm.DB, clientUUID string) error {
	var client models.Client
	if err := tx.Select("uuid").Where("uuid = ?", clientUUID).First(&client).Error; err != nil {
		return err
	}

	if err := tx.Where("client = ?", clientUUID).Delete(&models.Record{}).Error; err != nil {
		return fmt.Errorf("delete records: %w", err)
	}
	if err := tx.Table("records_long_term").Where("client = ?", clientUUID).Delete(&models.Record{}).Error; err != nil {
		return fmt.Errorf("delete records_long_term: %w", err)
	}
	if err := tx.Table("history_records").Where("client = ?", clientUUID).Delete(&models.Record{}).Error; err != nil {
		return fmt.Errorf("delete history_records: %w", err)
	}
	if err := tx.Where("client = ?", clientUUID).Delete(&models.PingRecord{}).Error; err != nil {
		return fmt.Errorf("delete ping_records: %w", err)
	}
	if err := tx.Where("client = ?", clientUUID).Delete(&models.TaskResult{}).Error; err != nil {
		return fmt.Errorf("delete task_results: %w", err)
	}

	var commandTasks []models.Task
	if err := tx.Find(&commandTasks).Error; err != nil {
		return err
	}
	for _, task := range commandTasks {
		remaining := make(models.StringArray, 0, len(task.Clients))
		for _, uuid := range task.Clients {
			if uuid != clientUUID {
				remaining = append(remaining, uuid)
			}
		}
		if len(remaining) == len(task.Clients) {
			continue
		}
		if len(remaining) == 0 {
			if err := tx.Where("task_id = ?", task.TaskId).Delete(&models.TaskResult{}).Error; err != nil {
				return err
			}
			if err := tx.Where("task_id = ?", task.TaskId).Delete(&models.Task{}).Error; err != nil {
				return err
			}
			continue
		}
		if err := tx.Model(&models.Task{}).Where("task_id = ?", task.TaskId).Update("clients", remaining).Error; err != nil {
			return err
		}
	}

	if err := tasks.RemoveClientFromPingTasksTx(tx, clientUUID); err != nil {
		return err
	}
	result := tx.Where("uuid = ?", clientUUID).Delete(&models.Client{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func SaveClientInfo(update map[string]interface{}) error {
	db := dbcore.GetDBInstance()
	clientUUID, ok := update["uuid"].(string)
	if !ok || clientUUID == "" {
		return fmt.Errorf("invalid client UUID")
	}

	// 确保更新的字段不为空
	if len(update) == 0 {
		return fmt.Errorf("no fields to update")
	}

	updatedAt, err := models.FromTime(time.Now()).Value()
	if err != nil {
		return err
	}
	update["updated_at"] = gorm.Expr("?", updatedAt)

	toFloat64 := func(value interface{}) (float64, bool) {
		switch typed := value.(type) {
		case float64:
			return typed, true
		case float32:
			return float64(typed), true
		case int:
			return float64(typed), true
		case int8:
			return float64(typed), true
		case int16:
			return float64(typed), true
		case int32:
			return float64(typed), true
		case int64:
			return float64(typed), true
		case uint:
			return float64(typed), true
		case uint8:
			return float64(typed), true
		case uint16:
			return float64(typed), true
		case uint32:
			return float64(typed), true
		case uint64:
			return float64(typed), true
		case json.Number:
			parsed, err := typed.Float64()
			if err != nil {
				return 0, false
			}
			return parsed, true
		default:
			return 0, false
		}
	}

	checkOptionalInt := func(name, key string, maxValue float64) error {
		value, exists := update[key]
		if !exists || value == nil {
			return nil
		}

		numericValue, ok := toFloat64(value)
		if !ok {
			return fmt.Errorf("%s must be a valid number", name)
		}
		if numericValue < 0 || numericValue > maxValue {
			return fmt.Errorf("%s must be a valid non-negative number: %v", name, value)
		}
		return nil
	}

	verify := func(update map[string]interface{}) error {
		if err := checkOptionalInt("Cpu.Cores", "cpu_cores", math.MaxInt-1); err != nil {
			return err
		}
		if err := checkOptionalInt("Cpu.PhysicalCores", "cpu_physical_cores", math.MaxInt-1); err != nil {
			return err
		}
		if err := checkOptionalInt("Ram.Total", "mem_total", math.MaxInt64-1); err != nil {
			return err
		}
		if err := checkOptionalInt("Swap.Total", "swap_total", math.MaxInt64-1); err != nil {
			return err
		}
		if err := checkOptionalInt("Disk.Total", "disk_total", math.MaxInt64-1); err != nil {
			return err
		}
		return nil
	}

	if err := verify(update); err != nil {
		return err
	}

	err = db.Model(&models.Client{}).Where("uuid = ?", clientUUID).Updates(update).Error
	if err != nil {
		return err
	}
	return nil
}

func EditClientName(clientUUID, clientName string) error {
	db := dbcore.GetDBInstance()
	err := db.Model(&models.Client{}).Where("uuid = ?", clientUUID).Update("name", clientName).Error
	if err != nil {
		return err
	}
	return nil
}

func EditClientToken(clientUUID, token string) error {
	db := dbcore.GetDBInstance()
	err := db.Model(&models.Client{}).Where("uuid = ?", clientUUID).Update("token", token).Error
	if err != nil {
		return err
	}
	return nil
}

// CreateClient 创建新客户端
func CreateClient() (clientUUID, token string, err error) {
	return createClient("", "")
}

func CreateClientWithName(name string) (clientUUID, token string, err error) {
	return createClient(name, "")
}

func CreateClientWithNameAndGroup(name, group string) (clientUUID, token string, err error) {
	return createClient(name, group)
}

func createClient(name, group string) (clientUUID, token string, err error) {
	// Serialize the read-minimum/create pair so concurrent additions cannot
	// receive the same leading weight. Client lists sort weight ascending, so a
	// value below the current minimum puts every newly created node first.
	clientCreationMu.Lock()
	defer clientCreationMu.Unlock()

	db := dbcore.GetDBInstance()
	token = utils.GenerateToken()
	clientUUID = uuid.New().String()
	name = strings.TrimSpace(name)
	if name == "" {
		name = "client_" + clientUUID[0:8]
	}
	now := time.Now()
	client := models.Client{
		UUID:                clientUUID,
		Token:               token,
		Name:                name,
		Group:               strings.TrimSpace(group),
		TrafficResetEnabled: true,
		PingTaskOrder:       models.UintArray{},
		CreatedAt:           models.FromTime(now),
		UpdatedAt:           models.FromTime(now),
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		var minimum sql.NullInt64
		if err := tx.Model(&models.Client{}).Select("MIN(weight)").Scan(&minimum).Error; err != nil {
			return err
		}
		if minimum.Valid {
			if minimum.Int64 <= int64(math.MinInt) {
				return fmt.Errorf("client weight range exhausted")
			}
			client.Weight = int(minimum.Int64 - 1)
		}
		return tx.Create(&client).Error
	})
	if err != nil {
		return "", "", err
	}
	return clientUUID, token, nil
}

/*
// GetAllClients 获取所有客户端配置

	func getAllClients() (clients []models.Client, err error) {
		db := dbcore.GetDBInstance()
		err = db.Find(&clients).Error
		if err != nil {
			return nil, err
		}
		return clients, nil
	}
*/
func GetClientByUUID(uuid string) (client models.Client, err error) {
	db := dbcore.GetDBInstance()
	err = db.Where("uuid = ?", uuid).First(&client).Error
	if err != nil {
		return models.Client{}, err
	}
	return client, nil
}

// GetClientBasicInfo 获取指定 UUID 的客户端基本信息
func GetClientBasicInfo(uuid string) (client models.Client, err error) {
	db := dbcore.GetDBInstance()
	err = db.Where("uuid = ?", uuid).First(&client).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return models.Client{}, fmt.Errorf("客户端不存在: %s", uuid)
		}
		return models.Client{}, err
	}
	return client, nil
}

func GetClientTokenByUUID(uuid string) (token string, err error) {
	db := dbcore.GetDBInstance()
	var client models.Client
	err = db.Where("uuid = ?", uuid).First(&client).Error
	if err != nil {
		return "", err
	}
	return client.Token, nil
}

func GetAllClientBasicInfo() (clients []models.Client, err error) {
	db := dbcore.GetDBInstance()
	err = db.Order("weight ASC, created_at ASC").Find(&clients).Error
	if err != nil {
		return nil, err
	}
	return clients, nil
}

// UpdateLastReportAt records the last time an agent report was accepted.
// This timestamp intentionally lives on the client row instead of in the
// rolling records tables, so it survives report-history cleanup and restarts.
func UpdateLastReportAt(clientUUID string, reportedAt time.Time) error {
	return updateLastReportAt(dbcore.GetDBInstance(), clientUUID, reportedAt)
}

func updateLastReportAt(db *gorm.DB, clientUUID string, reportedAt time.Time) error {
	if strings.TrimSpace(clientUUID) == "" {
		return fmt.Errorf("invalid client UUID")
	}
	if reportedAt.IsZero() {
		return fmt.Errorf("invalid report time")
	}

	result := db.Model(&models.Client{}).
		Where("uuid = ?", clientUUID).
		UpdateColumn("last_report_at", models.FromTime(reportedAt))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func SaveClient(updates map[string]interface{}) error {
	return saveClient(dbcore.GetDBInstance(), updates, time.Now())
}

func saveClient(db *gorm.DB, updates map[string]interface{}, now time.Time) error {
	clientUUID, ok := updates["uuid"].(string)
	if !ok || clientUUID == "" {
		return fmt.Errorf("invalid client UUID")
	}
	for _, removed := range []string{
		"traffic_compensation",
		"traffic_compensation_base",
		"traffic_compensation_reset_at",
	} {
		delete(updates, removed)
	}

	// 确保更新的字段不为空
	if len(updates) == 0 {
		return fmt.Errorf("no fields to update")
	}

	toInt64 := func(value interface{}) (int64, bool) {
		switch typed := value.(type) {
		case float64:
			if math.IsNaN(typed) || math.IsInf(typed, 0) || math.Trunc(typed) != typed ||
				typed < -9223372036854775808.0 || typed >= 9223372036854775808.0 {
				return 0, false
			}
			return int64(typed), true
		case float32:
			numeric := float64(typed)
			if math.IsNaN(numeric) || math.IsInf(numeric, 0) || math.Trunc(numeric) != numeric ||
				numeric < -9223372036854775808.0 || numeric >= 9223372036854775808.0 {
				return 0, false
			}
			return int64(numeric), true
		case int:
			return int64(typed), true
		case int8:
			return int64(typed), true
		case int16:
			return int64(typed), true
		case int32:
			return int64(typed), true
		case int64:
			return typed, true
		case uint:
			if uint64(typed) > math.MaxInt64 {
				return 0, false
			}
			return int64(typed), true
		case uint8:
			return int64(typed), true
		case uint16:
			return int64(typed), true
		case uint32:
			return int64(typed), true
		case uint64:
			if typed > math.MaxInt64 {
				return 0, false
			}
			return int64(typed), true
		case json.Number:
			parsed, err := typed.Int64()
			return parsed, err == nil
		default:
			return 0, false
		}
	}

	validateInteger := func(key string, min, max int64) error {
		value, exists := updates[key]
		if !exists {
			return nil
		}
		numeric, ok := toInt64(value)
		if !ok {
			return fmt.Errorf("%s must be a valid integer, got %v", key, value)
		}
		if numeric < min || numeric > max {
			return fmt.Errorf("%s must be between %v and %v, got %v", key, min, max, value)
		}
		return nil
	}

	if err := validateInteger("traffic_limit", 0, math.MaxInt64); err != nil {
		return err
	}
	if err := validateInteger("traffic_reset_day", 1, 31); err != nil {
		return err
	}
	if err := validateInteger("traffic_reset_hour", 0, 23); err != nil {
		return err
	}
	if err := validateInteger("traffic_reset_minute", 0, 59); err != nil {
		return err
	}
	updatedAt, err := models.FromTime(now).Value()
	if err != nil {
		return err
	}
	updates["updated_at"] = gorm.Expr("?", updatedAt)

	result := db.Model(&models.Client{}).Where("uuid = ?", clientUUID).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
