package records

import (
	"errors"
	"log"
	"sort"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/monitor-monitor/monitor/cmd/flags"
	"github.com/monitor-monitor/monitor/database/dbcore"
	"github.com/monitor-monitor/monitor/database/models"
	"github.com/monitor-monitor/monitor/utils"
)

const longTermRecordInterval = 15 * time.Minute

func DeleteAll() error {
	db := dbcore.GetDBInstance()
	if err := db.Exec("DELETE FROM records_long_term").Error; err != nil {
		return err
	}
	if err := db.Exec("DELETE FROM records").Error; err != nil {
		return err
	}
	return DeleteAllHistory()
}

func DeleteRecordBefore(before time.Time) error {
	return deleteLegacyRecordsBefore(dbcore.GetDBInstance(), before, time.Now())
}

func deleteLegacyRecordsBefore(db *gorm.DB, before, now time.Time) error {
	return db.Transaction(func(tx *gorm.DB) error {
		// Raw records are only the high-resolution history. Traffic that is old
		// enough to reach this cutoff has already been rolled up by CompactRecord.
		if err := tx.Table("records").Where("time < ?", before).Delete(&models.Record{}).Error; err != nil {
			return err
		}

		var clients []models.Client
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("uuid", "traffic_reset_enabled", "traffic_reset_day", "traffic_reset_hour", "traffic_reset_minute", "traffic_cleared_at").
			Find(&clients).Error; err != nil {
			return err
		}

		clientUUIDs := make([]string, 0, len(clients))
		for _, client := range clients {
			clientUUIDs = append(clientUUIDs, client.UUID)

			// Fold traffic deltas that still belong to the active billing window
			// into the internal carry before deleting old chart history.
			// This keeps cumulative traffic stable while allowing every historical
			// record row (CPU/RAM/etc.) older than seven days to be removed without
			// changing the user-managed traffic compensation field.
			foldStart := time.Time{}
			if client.TrafficResetEnabled {
				foldStart, _ = TrafficWindow(client, now)
			}
			foldStart = trafficAccountingStart(client, foldStart)
			var folded struct {
				Up   int64
				Down int64
			}
			foldQuery := tx.Table("records_long_term").
				Select("COALESCE(SUM(CASE WHEN traffic_up > 0 THEN traffic_up ELSE 0 END), 0) AS up, COALESCE(SUM(CASE WHEN traffic_down > 0 THEN traffic_down ELSE 0 END), 0) AS down").
				Where("client = ? AND time < ?", client.UUID, before)
			if !foldStart.IsZero() {
				foldQuery = foldQuery.Where("time >= ?", foldStart)
			}
			if err := foldQuery.Scan(&folded).Error; err != nil {
				return err
			}
			if folded.Up > 0 || folded.Down > 0 {
				if err := tx.Model(&models.Client{}).Where("uuid = ?", client.UUID).Updates(map[string]interface{}{
					"traffic_carry_up":              gorm.Expr("traffic_carry_up + ?", folded.Up),
					"traffic_carry_down":            gorm.Expr("traffic_carry_down + ?", folded.Down),
					"traffic_compensation_reset_at": now,
				}).Error; err != nil {
					return err
				}
			}
			if err := tx.Table("records_long_term").
				Where("client = ? AND time < ?", client.UUID, before).
				Delete(&models.Record{}).Error; err != nil {
				return err
			}
		}

		// Records belonging to deleted nodes no longer contribute to any total.
		orphanQuery := tx.Table("records_long_term").Where("time < ?", before)
		if len(clientUUIDs) > 0 {
			orphanQuery = orphanQuery.Where("client NOT IN ?", clientUUIDs)
		}
		return orphanQuery.Delete(&models.Record{}).Error
	})
}

func GetRecordsByClientAndTime(uuid string, start, end time.Time) ([]models.Record, error) {
	db := dbcore.GetDBInstance()
	var records []models.Record

	fourHoursAgo := time.Now().Add(-4*time.Hour - time.Minute)

	var recentRecords []models.Record
	recentStart := start
	if end.After(fourHoursAgo) {
		if recentStart.Before(fourHoursAgo) {
			recentStart = fourHoursAgo
		}
		err := db.Where("client = ? AND time >= ? AND time <= ?", uuid, recentStart, end).Order("time ASC").Find(&recentRecords).Error
		if err != nil {
			log.Printf("Error fetching recent records for client %s between %s and %s: %v", uuid, recentStart, end, err)
			return nil, err
		}
	}

	var long_term []models.Record
	err := db.Table("records_long_term").Where("client = ? AND time >= ? AND time <= ?", uuid, start, end).Order("time ASC").Find(&long_term).Error
	if err != nil {
		log.Printf("Error fetching long-term records for client %s between %s and %s: %v", uuid, start, end, err)
		return recentRecords, nil
	}

	if len(long_term) == 0 {
		// 没有查到long_term，返回全部recentRecords
		records = append(records, recentRecords...)
		return records, nil
	}

	// 查到了long_term，recentRecords按15分钟分组，每组只保留一条（取最新一条）
	grouped := make(map[string]models.Record)
	for _, rec := range recentRecords {
		key := rec.Time.ToTime().Truncate(15 * time.Minute).Format(time.RFC3339)
		if old, ok := grouped[key]; !ok || rec.Time.ToTime().After(old.Time.ToTime()) {
			grouped[key] = rec
		}
	}
	var groupedList []models.Record
	for _, rec := range grouped {
		groupedList = append(groupedList, rec)
	}
	sort.Slice(groupedList, func(i, j int) bool {
		return groupedList[i].Time.ToTime().Before(groupedList[j].Time.ToTime())
	})
	records = append(records, groupedList...)
	records = append(records, long_term...)
	return records, nil
}

// GetRecordsByTime 获取所有客户端在时间范围内的记录
func GetRecordsByTime(start, end time.Time) ([]models.Record, error) {
	db := dbcore.GetDBInstance()
	fourHoursAgo := time.Now().Add(-4*time.Hour - time.Minute)

	var recent []models.Record
	recentStart := start
	if end.After(fourHoursAgo) {
		if recentStart.Before(fourHoursAgo) {
			recentStart = fourHoursAgo
		}
		_ = db.Table("records").Where("time >= ? AND time <= ?", recentStart, end).Order("time ASC").Find(&recent).Error
	}

	var longTerm []models.Record
	_ = db.Table("records_long_term").Where("time >= ? AND time <= ?", start, end).Order("time ASC").Find(&longTerm).Error

	if len(longTerm) == 0 {
		return recent, nil
	}

	// group recent by client+15min, keep latest in bucket
	type key struct {
		c    string
		slot string
	}
	grouped := make(map[key]models.Record)
	for _, rec := range recent {
		k := key{c: rec.Client, slot: rec.Time.ToTime().Truncate(15 * time.Minute).Format(time.RFC3339)}
		if old, ok := grouped[k]; !ok || rec.Time.ToTime().After(old.Time.ToTime()) {
			grouped[k] = rec
		}
	}
	flat := make([]models.Record, 0, len(grouped))
	for _, rec := range grouped {
		flat = append(flat, rec)
	}
	sort.Slice(flat, func(i, j int) bool { return flat[i].Time.ToTime().Before(flat[j].Time.ToTime()) })
	flat = append(flat, longTerm...)
	return flat, nil
}

// CompactRecord compacts the primary SQLite history into 15-minute records.
func CompactRecord() error {
	db := dbcore.GetDBInstance()
	err := migrateOldRecords(db)
	if err != nil {
		log.Printf("Error migrating old records: %v", err)
		return err
	}

	if flags.IsSQLite() {
		db.Exec("PRAGMA wal_checkpoint(PASSIVE);")
	}
	//log.Printf("Record compaction completed")
	return nil
}

func migrateOldRecords(db *gorm.DB) error {
	return migrateOldRecordsAt(db, time.Now())
}

func compactRecordCutoff(now time.Time) time.Time {
	return now.Add(-4 * time.Hour).Truncate(longTermRecordInterval)
}

func migrateOldRecordsAt(db *gorm.DB, now time.Time) error {
	cutoff := compactRecordCutoff(now)

	// 查询 records 表中超过 4 小时的记录
	var records []models.Record
	if err := db.Table("records").Where("time < ?", cutoff).Find(&records).Error; err != nil {
		return err
	}

	if len(records) == 0 {
		return nil
	}
	previousByClient, err := getPreviousTrafficRecordsBefore(db, records)
	if err != nil {
		return err
	}
	trafficClearBaselines, err := getTrafficClearBaselines(db, records)
	if err != nil {
		return err
	}
	repairZeroTrafficDeltasWithBaselines(records, previousByClient, trafficClearBaselines)
	trafficClearTimes := make(map[string]time.Time, len(trafficClearBaselines))
	for clientUUID, baseline := range trafficClearBaselines {
		trafficClearTimes[clientUUID] = baseline.At
	}

	// 按 Client 和 15 分钟时间段分组，并存储所有记录以计算分位数
	type groupKey struct {
		Client string
		Slot   time.Time
	}
	type groupData struct {
		Cpu             []float32
		Load            []float32
		Temp            []float32
		Ram             []int64
		RamTotal        []int64
		Swap            []int64
		SwapTotal       []int64
		Disk            []int64
		DiskTotal       []int64
		NetIn           []int64
		NetOut          []int64
		NetTotalUp      []int64
		NetTotalDown    []int64
		TrafficUp       int64
		TrafficDown     int64
		LatestTime      time.Time
		LatestTotalUp   int64
		LatestTotalDown int64
		Process         []int
		Connections     []int
		ConnectionsUdp  []int
		Uptime          []int64
	}

	groupedRecords := make(map[groupKey]*groupData)
	for _, record := range records {
		recordTime := record.Time.ToTime()
		key := groupKey{
			Client: record.Client,
			Slot:   trafficCompactionSlot(recordTime, trafficClearTimes[record.Client]),
		}
		if _, ok := groupedRecords[key]; !ok {
			groupedRecords[key] = &groupData{}
		}
		data := groupedRecords[key]
		data.Cpu = append(data.Cpu, record.Cpu)
		data.Load = append(data.Load, record.Load)
		data.Temp = append(data.Temp, record.Temp)
		data.Ram = append(data.Ram, record.Ram)
		data.RamTotal = append(data.RamTotal, record.RamTotal)
		data.Swap = append(data.Swap, record.Swap)
		data.SwapTotal = append(data.SwapTotal, record.SwapTotal)
		data.Disk = append(data.Disk, record.Disk)
		data.DiskTotal = append(data.DiskTotal, record.DiskTotal)
		data.NetIn = append(data.NetIn, record.NetIn)
		data.NetOut = append(data.NetOut, record.NetOut)
		data.NetTotalUp = append(data.NetTotalUp, record.NetTotalUp)
		data.NetTotalDown = append(data.NetTotalDown, record.NetTotalDown)
		data.TrafficUp += record.TrafficUp
		data.TrafficDown += record.TrafficDown
		if data.LatestTime.IsZero() || record.Time.ToTime().After(data.LatestTime) {
			data.LatestTime = record.Time.ToTime()
			data.LatestTotalUp = record.NetTotalUp
			data.LatestTotalDown = record.NetTotalDown
		}
		data.Process = append(data.Process, record.Process)
		data.Connections = append(data.Connections, record.Connections)
		data.ConnectionsUdp = append(data.ConnectionsUdp, record.ConnectionsUdp)
		//data.Uptime = append(data.Uptime, record.Uptime)
	}

	getPercentile := func(values []float64, percentile float64) float64 {
		if len(values) == 0 {
			return 0
		}
		sortedValues := make([]float64, len(values))
		copy(sortedValues, values)
		sort.Float64s(sortedValues)
		index := float64(len(sortedValues)-1) * percentile
		lowerIndex := int(index)
		if lowerIndex >= len(sortedValues)-1 {
			return sortedValues[len(sortedValues)-1]
		}
		frac := index - float64(lowerIndex)
		return sortedValues[lowerIndex] + frac*(sortedValues[lowerIndex+1]-sortedValues[lowerIndex])
	}

	getIntPercentile := func(values []int64, percentile float64) int64 {
		if len(values) == 0 {
			return 0
		}
		floats := make([]float64, len(values))
		for i, v := range values {
			floats[i] = float64(v)
		}
		return int64(getPercentile(floats, percentile))
	}

	getInt32Percentile := func(values []int, percentile float64) int {
		if len(values) == 0 {
			return 0
		}
		floats := make([]float64, len(values))
		for i, v := range values {
			floats[i] = float64(v)
		}
		return int(getPercentile(floats, percentile))
	}

	return db.Transaction(func(tx *gorm.DB) error {
		for key, data := range groupedRecords {
			clientUUID := key.Client
			timeSlot := key.Slot

			cpuFloats := make([]float64, len(data.Cpu))
			for i, v := range data.Cpu {
				cpuFloats[i] = float64(v)
			}
			loadFloats := make([]float64, len(data.Load))
			for i, v := range data.Load {
				loadFloats[i] = float64(v)
			}
			tempFloats := make([]float64, len(data.Temp))
			for i, v := range data.Temp {
				tempFloats[i] = float64(v)
			}
			// 取高位
			high_percentile := 0.7
			// 检查 records_long_term 表中是否已存在相同的记录
			// 必须使用 models.FromTime() 转换，因为数据库存储的是格式化后的字符串
			var existingCount int64
			if err := tx.Table("records_long_term").Where("client = ? AND time = ?", clientUUID, models.FromTime(timeSlot)).Count(&existingCount).Error; err != nil {
				return err
			}

			newRec := models.Record{
				Client:         clientUUID,
				Time:           models.FromTime(timeSlot),
				Cpu:            float32(getPercentile(cpuFloats, high_percentile)),
				Load:           float32(getPercentile(loadFloats, high_percentile)),
				Temp:           float32(getPercentile(tempFloats, high_percentile)),
				Ram:            getIntPercentile(data.Ram, high_percentile),
				RamTotal:       getIntPercentile(data.RamTotal, high_percentile),
				Swap:           getIntPercentile(data.Swap, high_percentile),
				SwapTotal:      getIntPercentile(data.SwapTotal, high_percentile),
				Disk:           getIntPercentile(data.Disk, high_percentile),
				DiskTotal:      getIntPercentile(data.DiskTotal, high_percentile),
				NetIn:          getIntPercentile(data.NetIn, 0.2),
				NetOut:         getIntPercentile(data.NetOut, 0.2),
				NetTotalUp:     data.LatestTotalUp,
				NetTotalDown:   data.LatestTotalDown,
				TrafficUp:      data.TrafficUp,
				TrafficDown:    data.TrafficDown,
				Process:        getInt32Percentile(data.Process, high_percentile),
				Connections:    getInt32Percentile(data.Connections, high_percentile),
				ConnectionsUdp: getInt32Percentile(data.ConnectionsUdp, high_percentile),
				//Uptime:         getIntPercentile(data.Uptime, high_percentile),
			}

			// 如果记录已存在则更新，否则创建新记录
			if existingCount > 0 {
				if err := tx.Table("records_long_term").Where("client = ? AND time = ?", clientUUID, models.FromTime(timeSlot)).Updates(&newRec).Error; err != nil {
					return err
				}
			} else {
				if err := tx.Table("records_long_term").Create(&newRec).Error; err != nil {
					return err
				}
			}
		}

		// 删除 records 表中的旧数据
		if err := tx.Table("records").Where("time < ?", cutoff.Add(-1*time.Hour)).Delete(&models.Record{}).Error; err != nil {
			return err
		}

		return nil
	})
}

type trafficClearBaseline struct {
	At   time.Time
	Up   int64
	Down int64
}

func getTrafficClearBaselines(db *gorm.DB, records []models.Record) (map[string]trafficClearBaseline, error) {
	baselines := make(map[string]trafficClearBaseline)
	if !db.Migrator().HasTable(&models.Client{}) {
		return baselines, nil
	}

	clientIDs := make([]string, 0)
	seen := make(map[string]struct{})
	for _, record := range records {
		if record.Client == "" {
			continue
		}
		if _, exists := seen[record.Client]; exists {
			continue
		}
		seen[record.Client] = struct{}{}
		clientIDs = append(clientIDs, record.Client)
	}
	if len(clientIDs) == 0 {
		return baselines, nil
	}

	var clients []models.Client
	if err := db.Select("uuid", "traffic_cleared_at", "traffic_baseline_up", "traffic_baseline_down").
		Where("uuid IN ?", clientIDs).
		Find(&clients).Error; err != nil {
		return nil, err
	}
	for _, client := range clients {
		clearedAt := client.TrafficClearedAt.ToTime()
		if clearedAt.IsZero() {
			continue
		}
		baselines[client.UUID] = trafficClearBaseline{
			At:   clearedAt,
			Up:   client.TrafficBaselineUp,
			Down: client.TrafficBaselineDown,
		}
	}
	return baselines, nil
}

// trafficCompactionSlot splits the one 15-minute bucket containing a manual
// traffic clear at the exact clear time. This keeps pre-clear deltas excluded
// while preserving every post-clear delta after raw history is compacted.
func trafficCompactionSlot(recordTime, clearedAt time.Time) time.Time {
	slot := recordTime.Truncate(longTermRecordInterval)
	if clearedAt.After(slot) &&
		clearedAt.Before(slot.Add(longTermRecordInterval)) &&
		!recordTime.Before(clearedAt) {
		return clearedAt
	}
	return slot
}

func getPreviousTrafficRecordsBefore(db *gorm.DB, records []models.Record) (map[string]*models.Record, error) {
	firstTimeByClient := make(map[string]time.Time)
	for _, record := range records {
		recordTime := record.Time.ToTime()
		firstTime, ok := firstTimeByClient[record.Client]
		if !ok || recordTime.Before(firstTime) {
			firstTimeByClient[record.Client] = recordTime
		}
	}

	previousByClient := make(map[string]*models.Record, len(firstTimeByClient))
	for clientUUID, firstTime := range firstTimeByClient {
		previous, err := getPreviousTrafficRecordBefore(db, clientUUID, firstTime)
		if err != nil {
			return nil, err
		}
		if previous != nil {
			previousByClient[clientUUID] = previous
		}
	}
	return previousByClient, nil
}

func getPreviousTrafficRecordBefore(db *gorm.DB, clientUUID string, before time.Time) (*models.Record, error) {
	var latest *models.Record
	for _, table := range []string{"records", "records_long_term"} {
		var record models.Record
		queryBefore := before
		if table == "records_long_term" {
			queryBefore = before.Truncate(15 * time.Minute)
		}
		err := db.Table(table).
			Where("client = ? AND time < ?", clientUUID, models.FromTime(queryBefore)).
			Order("time DESC").
			First(&record).Error
		if err == nil {
			if latest == nil || record.Time.ToTime().After(latest.Time.ToTime()) {
				latest = &record
			}
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	return latest, nil
}

func repairZeroTrafficDeltas(records []models.Record, previousByClient map[string]*models.Record) {
	repairZeroTrafficDeltasWithBaselines(records, previousByClient, nil)
}

func repairZeroTrafficDeltasWithBaselines(
	records []models.Record,
	previousByClient map[string]*models.Record,
	baselines map[string]trafficClearBaseline,
) {
	recordsByClient := make(map[string][]*models.Record)
	for i := range records {
		recordsByClient[records[i].Client] = append(recordsByClient[records[i].Client], &records[i])
	}

	for _, clientRecords := range recordsByClient {
		sort.Slice(clientRecords, func(i, j int) bool {
			return clientRecords[i].Time.ToTime().Before(clientRecords[j].Time.ToTime())
		})
		var previous *models.Record
		if previousByClient != nil {
			previous = previousByClient[clientRecords[0].Client]
		}
		for _, current := range clientRecords {
			if baseline, ok := baselines[current.Client]; ok &&
				!current.Time.ToTime().Before(baseline.At) &&
				(previous == nil || previous.Time.ToTime().Before(baseline.At)) {
				previous = &models.Record{
					Client:       current.Client,
					Time:         models.FromTime(baseline.At),
					NetTotalUp:   baseline.Up,
					NetTotalDown: baseline.Down,
				}
			}
			if previous == nil {
				previous = current
				continue
			}
			if current.TrafficUp == 0 {
				current.TrafficUp = utils.ComputeTrafficDelta(current.NetTotalUp, previous.NetTotalUp)
			}
			if current.TrafficDown == 0 {
				current.TrafficDown = utils.ComputeTrafficDelta(current.NetTotalDown, previous.NetTotalDown)
			}
			previous = current
		}
	}
}
