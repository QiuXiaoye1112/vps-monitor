package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/monitor-monitor/monitor/database/dbcore"
	"github.com/monitor-monitor/monitor/database/metricstore"
	"github.com/monitor-monitor/monitor/database/models"
	"github.com/monitor-monitor/monitor/database/tasks"
	"github.com/monitor-monitor/monitor/utils"
	"gorm.io/gorm"

	"github.com/google/uuid"
)

func DeleteClient(clientUuid string) error {
	if strings.TrimSpace(clientUuid) == "" {
		return fmt.Errorf("invalid client UUID")
	}

	// The metric store is a separate database and cannot participate in the
	// GORM transaction below. Delete it first so a metric-store error cannot
	// leave a deleted node's history behind.
	if err := metricstore.DeleteEntity(context.Background(), clientUuid); err != nil {
		return fmt.Errorf("delete client metrics: %w", err)
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

	update["updated_at"] = time.Now()

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

	err := db.Model(&models.Client{}).Where("uuid = ?", clientUUID).Updates(update).Error
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
	db := dbcore.GetDBInstance()
	token = utils.GenerateToken()
	clientUUID = uuid.New().String()
	name = strings.TrimSpace(name)
	if name == "" {
		name = "client_" + clientUUID[0:8]
	}
	client := models.Client{
		UUID:                clientUUID,
		Token:               token,
		Name:                name,
		Group:               strings.TrimSpace(group),
		TrafficResetEnabled: true,
		PingTaskOrder:       models.UintArray{},
		CreatedAt:           models.FromTime(time.Now()),
		UpdatedAt:           models.FromTime(time.Now()),
	}

	err = db.Create(&client).Error
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

// ResetTrafficCompensationForDueClients 检查所有节点，若已进入新的流量计费周期则将
// 用户补偿 traffic_compensation 与上下行内部结转一并清零。
// 判断依据：以当前时间为准，用 TrafficWindow 算出本周期起始时间；若 client 的 traffic_compensation_reset_at
// 早于该起始时间，且补偿或内部结转不为 0，则视为上一周期的值并清零。
func ResetTrafficCompensationForDueClients() {
	db := dbcore.GetDBInstance()
	allClients, err := GetAllClientBasicInfo()
	if err != nil {
		log.Printf("[traffic_comp_reset] failed to get clients: %v", err)
		return
	}
	now := time.Now()
	for _, c := range allClients {
		if !shouldResetTrafficCompensation(c, now) {
			continue
		}
		if err := db.Model(&models.Client{}).Where("uuid = ?", c.UUID).
			Updates(map[string]interface{}{
				"traffic_compensation":          int64(0),
				"traffic_carry":                 int64(0), // Clear any pre-v3 residue as well.
				"traffic_carry_up":              int64(0),
				"traffic_carry_down":            int64(0),
				"traffic_compensation_reset_at": now,
				"updated_at":                    now,
			}).Error; err != nil {
			log.Printf("[traffic_comp_reset] failed to reset comp for %s: %v", c.UUID, err)
		} else {
			log.Printf(
				"[traffic_comp_reset] reset traffic accounting for %s (compensation=%d, carry_up=%d, carry_down=%d)",
				c.UUID,
				c.TrafficComp,
				c.TrafficCarryUp,
				c.TrafficCarryDown,
			)
		}
	}
}

func shouldResetTrafficCompensation(client models.Client, now time.Time) bool {
	if (client.TrafficComp == 0 &&
		client.TrafficCarry == 0 &&
		client.TrafficCarryUp == 0 &&
		client.TrafficCarryDown == 0) ||
		!client.TrafficResetEnabled {
		return false
	}
	start := trafficResetStart(client, now)
	compResetTime := client.TrafficCompResetAt.ToTime()
	if compResetTime.IsZero() {
		compResetTime = client.CreatedAt.ToTime()
	}
	return compResetTime.Before(start)
}

// trafficResetStart 返回当前计费周期的起始时间（Asia/Shanghai 时区）。
func trafficResetStart(client models.Client, now time.Time) time.Time {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	localNow := now.In(loc)
	day := client.TrafficResetDay
	if day <= 0 {
		day = 1
	}
	hour := client.TrafficResetHour
	if hour < 0 || hour > 23 {
		hour = 0
	}
	// 本月重置时间点
	lastDayOfMonth := time.Date(localNow.Year(), localNow.Month()+1, 0, 0, 0, 0, 0, loc).Day()
	resetDay := day
	if resetDay > lastDayOfMonth {
		resetDay = lastDayOfMonth
	}
	thisReset := time.Date(localNow.Year(), localNow.Month(), resetDay, hour, 0, 0, 0, loc)
	if localNow.Before(thisReset) {
		// 当前时刻还没到本月重置点，周期起点是上个月的重置点
		prevYear, prevMonth := localNow.Year(), localNow.Month()-1
		if prevMonth == 0 {
			prevMonth = 12
			prevYear--
		}
		lastDayPrev := time.Date(prevYear, prevMonth+1, 0, 0, 0, 0, 0, loc).Day()
		prevDay := day
		if prevDay > lastDayPrev {
			prevDay = lastDayPrev
		}
		return time.Date(prevYear, prevMonth, prevDay, hour, 0, 0, 0, loc)
	}
	return thisReset
}

func SaveClient(updates map[string]interface{}) error {
	db := dbcore.GetDBInstance()
	clientUUID, ok := updates["uuid"].(string)
	if !ok || clientUUID == "" {
		return fmt.Errorf("invalid client UUID")
	}

	// 确保更新的字段不为空
	if len(updates) == 0 {
		return fmt.Errorf("no fields to update")
	}

	if v, exists := updates["traffic_limit"]; exists {
		if val, ok := v.(float64); ok {
			if val < 0 || val > math.MaxInt64-1 {
				return fmt.Errorf("traffic_limit must be a valid non-negative int64 value, got %v", val)
			}
		}
	}
	if v, exists := updates["traffic_reset_day"]; exists {
		if val, ok := v.(float64); ok {
			if val < 1 || val > 31 {
				return fmt.Errorf("traffic_reset_day must be between 1 and 31, got %v", val)
			}
		}
	}
	if v, exists := updates["traffic_reset_hour"]; exists {
		if val, ok := v.(float64); ok {
			if val < 0 || val > 23 {
				return fmt.Errorf("traffic_reset_hour must be between 0 and 23, got %v", val)
			}
		}
	}
	if v, exists := updates["traffic_compensation"]; exists {
		if val, ok := v.(float64); ok {
			if val < -math.MaxInt64 || val > math.MaxInt64-1 {
				return fmt.Errorf("traffic_compensation must be a valid int64 value, got %v", val)
			}
		}
	}

	if _, exists := updates["traffic_compensation"]; exists {
		updates["traffic_compensation_reset_at"] = time.Now()
	}

	updates["updated_at"] = time.Now()

	err := db.Model(&models.Client{}).Where("uuid = ?", clientUUID).Updates(updates).Error
	if err != nil {
		return err
	}
	return nil
}
