package tasks

import (
	"fmt"
	"time"

	"github.com/monitor-monitor/monitor/database/dbcore"
	"github.com/monitor-monitor/monitor/database/models"
	"github.com/monitor-monitor/monitor/utils"
	"gorm.io/gorm"
)

// AddPingTask 创建延迟监测任务。defaultOn 表示新加入的服务器是否自动开启此监测。
func AddPingTask(clients []string, defaultOn, enabled bool, name string, target, task_type string, interval int) (uint, error) {
	db := dbcore.GetDBInstance()
	normalizedClients := normalizePingClients(models.StringArray(clients))
	task := models.PingTask{
		Clients:   normalizedClients,
		DefaultOn: defaultOn,
		Enabled:   enabled,
		Name:      name,
		Type:      task_type,
		Target:    target,
		Interval:  interval,
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&task).Error; err != nil {
			return err
		}

		// Append by id to avoid races between concurrent create requests.
		result := tx.Model(&models.PingTask{}).Where("id = ?", task.Id).Update("weight", int(task.Id))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}

		return nil
	})
	if err != nil {
		return 0, err
	}
	ReloadPingSchedule()
	return task.Id, nil
}

func DeletePingTask(id []uint) error {
	db := dbcore.GetDBInstance()
	err := db.Transaction(func(tx *gorm.DB) error { return deletePingTasksTx(tx, id) })
	if err != nil {
		return err
	}
	ReloadPingSchedule()
	return nil
}

func deletePingTasksTx(tx *gorm.DB, id []uint) error {
	removed := make(map[uint]struct{}, len(id))
	for _, taskID := range id {
		removed[taskID] = struct{}{}
	}
	result := tx.Where("id IN ?", id).Delete(&models.PingTask{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	var clients []models.Client
	if err := tx.Select("uuid", "ping_task_order").Find(&clients).Error; err != nil {
		return err
	}
	for _, client := range clients {
		filtered := make(models.UintArray, 0, len(client.PingTaskOrder))
		for _, taskID := range client.PingTaskOrder {
			if _, deleted := removed[taskID]; !deleted {
				filtered = append(filtered, taskID)
			}
		}
		if len(filtered) != len(client.PingTaskOrder) {
			if err := tx.Model(&models.Client{}).Where("uuid = ?", client.UUID).Update("ping_task_order", filtered).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// EditPingTask 批量更新延迟监测任务配置。
func EditPingTask(tasks []*models.PingTask) error {
	db := dbcore.GetDBInstance()
	for _, task := range tasks {
		task.Clients = normalizePingClients(task.Clients)
		// 使用 map 显式更新，避免 GORM struct Updates 跳过 false/0/空切片等零值。
		updates := map[string]interface{}{
			"name":        task.Name,
			"clients":     task.Clients,
			"all_clients": task.DefaultOn,
			"enabled":     task.Enabled,
			"type":        task.Type,
			"target":      task.Target,
			"interval":    task.Interval,
		}
		result := db.Model(&models.PingTask{}).Where("id = ?", task.Id).Updates(updates)
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
	}
	ReloadPingSchedule()
	return nil
}

// normalizePingClients 保持 clients 字段序列化为 JSON 数组，避免空值变成 null。
func normalizePingClients(clients models.StringArray) models.StringArray {
	if clients == nil {
		return models.StringArray{}
	}
	return clients
}

func GetAllPingTasks() ([]models.PingTask, error) {
	db := dbcore.GetDBInstance()
	var tasks []models.PingTask
	if err := db.Order("weight ASC").Order("id ASC").Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}

// GetPingTasksByClient 获取指定服务器需要执行的延迟监测任务。
func GetPingTasksByClient(uuid string) []models.PingTask {
	db := dbcore.GetDBInstance()
	var client models.Client
	if err := db.Select("uuid", "ping_task_order").Where("uuid = ?", uuid).First(&client).Error; err != nil {
		return nil
	}
	if len(client.PingTaskOrder) == 0 {
		return []models.PingTask{}
	}
	var pingTasks []models.PingTask
	if err := db.Where("enabled = ? AND id IN ?", true, []uint(client.PingTaskOrder)).Find(&pingTasks).Error; err != nil {
		return nil
	}
	return OrderPingTasks(client.PingTaskOrder, pingTasks)
}

// GetClientPingTaskOrder returns the node-owned task order.
func GetClientPingTaskOrder(uuid string) models.UintArray {
	if uuid == "" {
		return models.UintArray{}
	}
	var client models.Client
	if err := dbcore.GetDBInstance().Select("uuid", "ping_task_order").Where("uuid = ?", uuid).First(&client).Error; err != nil {
		return models.UintArray{}
	}
	return client.PingTaskOrder
}

// OrderPingTasks filters candidates to the selected IDs and returns them in
// the exact order configured on the node.
func OrderPingTasks(order models.UintArray, candidates []models.PingTask) []models.PingTask {
	byID := make(map[uint]models.PingTask, len(candidates))
	for _, task := range candidates {
		byID[task.Id] = task
	}
	ordered := make([]models.PingTask, 0, len(order))
	for _, taskID := range order {
		if task, ok := byID[taskID]; ok {
			ordered = append(ordered, task)
		}
	}
	return ordered
}

// SetClientPingTaskOrder atomically updates the node-owned ordered selection
// and mirrors it into PingTask.Clients for compatibility with record filters.
func SetClientPingTaskOrder(uuid string, taskIDs []uint) error {
	if uuid == "" {
		return fmt.Errorf("invalid client UUID")
	}
	normalized := make(models.UintArray, 0, len(taskIDs))
	seen := make(map[uint]struct{}, len(taskIDs))
	for _, taskID := range taskIDs {
		if taskID == 0 {
			return fmt.Errorf("invalid ping task ID: 0")
		}
		if _, exists := seen[taskID]; exists {
			continue
		}
		seen[taskID] = struct{}{}
		normalized = append(normalized, taskID)
	}

	db := dbcore.GetDBInstance()
	err := db.Transaction(func(tx *gorm.DB) error {
		var client models.Client
		if err := tx.Select("uuid").Where("uuid = ?", uuid).First(&client).Error; err != nil {
			return err
		}
		var pingTasks []models.PingTask
		if err := tx.Order("weight ASC").Order("id ASC").Find(&pingTasks).Error; err != nil {
			return err
		}
		valid := make(map[uint]struct{}, len(pingTasks))
		for _, task := range pingTasks {
			valid[task.Id] = struct{}{}
		}
		for _, taskID := range normalized {
			if _, ok := valid[taskID]; !ok {
				return fmt.Errorf("ping task not found: %d", taskID)
			}
		}
		if err := tx.Model(&models.Client{}).Where("uuid = ?", uuid).Update("ping_task_order", normalized).Error; err != nil {
			return err
		}
		for _, task := range pingTasks {
			clients := normalizePingClients(task.Clients)
			updated := make(models.StringArray, 0, len(clients)+1)
			for _, clientUUID := range clients {
				if clientUUID != uuid {
					updated = append(updated, clientUUID)
				}
			}
			if _, selected := seen[task.Id]; selected {
				updated = append(updated, uuid)
			}
			if err := tx.Model(&models.PingTask{}).Where("id = ?", task.Id).Updates(map[string]interface{}{
				"clients":     updated,
				"all_clients": false,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return ReloadPingSchedule()
}

// RemoveClientFromPingTasks removes a deleted node from compatibility scopes.
func RemoveClientFromPingTasks(uuid string) error {
	if uuid == "" {
		return nil
	}
	db := dbcore.GetDBInstance()
	err := db.Transaction(func(tx *gorm.DB) error { return RemoveClientFromPingTasksTx(tx, uuid) })
	if err != nil {
		return err
	}
	return ReloadPingSchedule()
}

// RemoveClientFromPingTasksTx removes a node from Ping task scopes using the
// caller's transaction. Historical Ping records are intentionally untouched;
// the seven-day retention cleanup removes them later.
func RemoveClientFromPingTasksTx(tx *gorm.DB, uuid string) error {
	if uuid == "" {
		return nil
	}
	var pingTasks []models.PingTask
	if err := tx.Find(&pingTasks).Error; err != nil {
		return err
	}
	for _, task := range pingTasks {
		updated := make(models.StringArray, 0, len(task.Clients))
		for _, clientUUID := range task.Clients {
			if clientUUID != uuid {
				updated = append(updated, clientUUID)
			}
		}
		if len(updated) != len(task.Clients) {
			if err := tx.Model(&models.PingTask{}).Where("id = ?", task.Id).Update("clients", updated).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func GetEnabledPingTasks() ([]models.PingTask, error) {
	db := dbcore.GetDBInstance()
	var tasks []models.PingTask
	if err := db.Where("enabled = ?", true).Order("weight ASC").Order("id ASC").Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}

func UpdatePingTaskOrder(order map[uint]int) error {
	db := dbcore.GetDBInstance()
	err := db.Transaction(func(tx *gorm.DB) error {
		for id, weight := range order {
			result := tx.Model(&models.PingTask{}).Where("id = ?", id).Update("weight", weight)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return gorm.ErrRecordNotFound
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	ReloadPingSchedule()
	return nil
}

func SavePingRecord(record models.PingRecord) error {
	db := dbcore.GetDBInstance()
	return db.Create(&record).Error
}

func DeletePingRecordsBefore(time time.Time) error {
	db := dbcore.GetDBInstance()
	err := db.Where("time < ?", time).Delete(&models.PingRecord{}).Error
	return err
}

func DeletePingRecords(id []uint) error {
	db := dbcore.GetDBInstance()
	result := db.Where("task_id IN ?", id).Delete(&models.PingRecord{})
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return result.Error
}

func DeleteAllPingRecords() error {
	db := dbcore.GetDBInstance()
	result := db.Exec("DELETE FROM ping_records")
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return result.Error
}
func ReloadPingSchedule() error {
	db := dbcore.GetDBInstance()
	var pingTasks []models.PingTask
	if err := db.Where("enabled = ?", true).Find(&pingTasks).Error; err != nil {
		return err
	}
	return utils.ReloadPingSchedule(pingTasks)
}

func GetPingRecords(uuid string, taskId int, start, end time.Time) ([]models.PingRecord, error) {
	db := dbcore.GetDBInstance()
	var records []models.PingRecord
	dbQuery := db.Model(&models.PingRecord{})
	if uuid != "" {
		dbQuery = dbQuery.Where("client = ?", uuid)
	}
	if taskId >= 0 {
		dbQuery = dbQuery.Where("task_id = ?", uint(taskId))
	}
	if err := dbQuery.Where("time >= ? AND time <= ?", start, end).Order("time DESC").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}
