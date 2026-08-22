package records

import (
	"log"
	"sort"
	"time"

	"gorm.io/gorm"

	"github.com/monitor-monitor/monitor/cmd/flags"
	"github.com/monitor-monitor/monitor/database/dbcore"
	"github.com/monitor-monitor/monitor/database/models"
)

const longTermRecordInterval = 15 * time.Minute

// applyLocalTimeRange binds bounds using the timezone-less storage format of
// models.LocalTime. It is shared by monitoring-history queries only.
func applyLocalTimeRange(query *gorm.DB, start, end time.Time) *gorm.DB {
	if !start.IsZero() {
		query = query.Where("time >= ?", models.FromTime(start))
	}
	if !end.IsZero() {
		query = query.Where("time <= ?", models.FromTime(end))
	}
	return query
}

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

func deleteLegacyRecordsBefore(db *gorm.DB, before, _ time.Time) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Table("records").Where("time < ?", models.FromTime(before)).Delete(&models.Record{}).Error; err != nil {
			return err
		}
		return tx.Table("records_long_term").Where("time < ?", models.FromTime(before)).Delete(&models.Record{}).Error
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
		err := applyLocalTimeRange(db.Where("client = ?", uuid), recentStart, end).
			Order("time ASC").Find(&recentRecords).Error
		if err != nil {
			log.Printf("Error fetching recent records for client %s between %s and %s: %v", uuid, recentStart, end, err)
			return nil, err
		}
	}

	var long_term []models.Record
	err := applyLocalTimeRange(db.Table("records_long_term").Where("client = ?", uuid), start, end).
		Order("time ASC").Find(&long_term).Error
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

// GetLatestRecordByClient returns the newest retained record for a client
// across the raw, compacted, and dedicated Agent history tables.
func GetLatestRecordByClient(uuid string) (*models.Record, error) {
	return getLatestRecordByClient(dbcore.GetDBInstance(), uuid)
}

func getLatestRecordByClient(db *gorm.DB, uuid string) (*models.Record, error) {
	if db == nil || uuid == "" {
		return nil, nil
	}

	var latest *models.Record
	for _, table := range []string{"records", "records_long_term", HistoryTable} {
		if !db.Migrator().HasTable(table) {
			continue
		}

		var rows []models.Record
		if err := db.Table(table).
			Where("client = ?", uuid).
			Order("time DESC").
			Limit(1).
			Find(&rows).Error; err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			continue
		}

		row := rows[0]
		if latest == nil || row.Time.ToTime().After(latest.Time.ToTime()) {
			latest = &row
		}
	}

	return latest, nil
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
		_ = applyLocalTimeRange(db.Table("records"), recentStart, end).
			Order("time ASC").Find(&recent).Error
	}

	var longTerm []models.Record
	_ = applyLocalTimeRange(db.Table("records_long_term"), start, end).
		Order("time ASC").Find(&longTerm).Error

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
	if err := db.Table("records").Where("time < ?", models.FromTime(cutoff)).Find(&records).Error; err != nil {
		return err
	}

	if len(records) == 0 {
		return nil
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
			Slot:   recordTime.Truncate(longTermRecordInterval),
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
		if err := tx.Table("records").Where("time < ?", models.FromTime(cutoff.Add(-1*time.Hour))).Delete(&models.Record{}).Error; err != nil {
			return err
		}

		return nil
	})
}
