package records

import (
	"math"
	"strconv"
	"time"

	"github.com/monitor-monitor/monitor/database/dbcore"
	"github.com/monitor-monitor/monitor/database/models"
	"gorm.io/gorm"
)

const HistoryTable = "history_records"

func SaveHistoryRecords(records []models.Record) error {
	if len(records) == 0 {
		return nil
	}
	return dbcore.GetDBInstance().Table(HistoryTable).CreateInBatches(records, 200).Error
}

func DeleteHistoryBefore(before time.Time) error {
	return dbcore.GetDBInstance().
		Table(HistoryTable).
		Where("time < ?", models.FromTime(before)).
		Delete(&models.Record{}).Error
}

func DeleteAllHistory() error {
	return dbcore.GetDBInstance().Exec("DELETE FROM " + HistoryTable).Error
}

// GetHistoryRecordsByClientAndTime returns raw Agent history samples whenever
// they fit in maxCount. Larger ranges are aggregated into time buckets so the
// browser receives progressively coarser data for progressively longer ranges.
func GetHistoryRecordsByClientAndTime(clientUUID string, start, end time.Time, maxCount int) ([]models.Record, error) {
	return getHistoryRecordsByClientAndTime(dbcore.GetDBInstance(), clientUUID, start, end, maxCount)
}

func getHistoryRecordsByClientAndTime(db *gorm.DB, clientUUID string, start, end time.Time, maxCount int) ([]models.Record, error) {
	query := db.Table(HistoryTable).
		Where("client = ? AND time >= ? AND time <= ?", clientUUID, models.FromTime(start), models.FromTime(end))

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		return []models.Record{}, nil
	}
	if maxCount <= 0 || count <= int64(maxCount) {
		var result []models.Record
		err := query.Order("time ASC").Find(&result).Error
		return result, err
	}

	rangeSeconds := math.Max(1, end.Sub(start).Seconds())
	bucketSeconds := int64(math.Ceil(rangeSeconds / float64(maxCount)))
	if bucketSeconds < 1 {
		bucketSeconds = 1
	}

	var result []models.Record
	err := db.Table(HistoryTable).
		Select(`
			client,
			MAX(time) AS time,
			AVG(cpu) AS cpu,
			AVG(gpu) AS gpu,
			CAST(AVG(ram) AS INTEGER) AS ram,
			MAX(ram_total) AS ram_total,
			CAST(AVG(swap) AS INTEGER) AS swap,
			MAX(swap_total) AS swap_total,
			AVG(load) AS load,
			AVG(temp) AS temp,
			CAST(AVG(disk) AS INTEGER) AS disk,
			MAX(disk_total) AS disk_total,
			CAST(AVG(net_in) AS INTEGER) AS net_in,
			CAST(AVG(net_out) AS INTEGER) AS net_out,
			MAX(net_total_up) AS net_total_up,
			MAX(net_total_down) AS net_total_down,
			CAST(ROUND(AVG(process)) AS INTEGER) AS process,
			CAST(ROUND(AVG(connections)) AS INTEGER) AS connections,
			CAST(ROUND(AVG(connections_udp)) AS INTEGER) AS connections_udp
		`).
		Where("client = ? AND time >= ? AND time <= ?", clientUUID, models.FromTime(start), models.FromTime(end)).
		Group("client, CAST(strftime('%s', time) AS INTEGER) / " + strconv.FormatInt(bucketSeconds, 10)).
		Order("time ASC").
		Scan(&result).Error
	return result, err
}
