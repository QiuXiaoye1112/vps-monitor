package report

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/monitor-monitor/monitor/database/clients"
	"github.com/monitor-monitor/monitor/database/dbcore"
	"github.com/monitor-monitor/monitor/database/metricstore"
	"github.com/monitor-monitor/monitor/database/models"
	"github.com/monitor-monitor/monitor/protocol/v1"
	"github.com/monitor-monitor/monitor/utils"
	"github.com/patrickmn/go-cache"
	"gorm.io/gorm"
)

var Records = cache.New(1*time.Minute, 1*time.Minute)
var reportCacheMu sync.Mutex
var saveClientReportMu sync.Mutex

// DeleteClientReports waits for an active persistence pass and then removes
// every buffered report for a deleted node, preventing it from being written
// back after the database cleanup transaction completes.
func DeleteClientReports(uuid string) {
	saveClientReportMu.Lock()
	defer saveClientReportMu.Unlock()
	reportCacheMu.Lock()
	defer reportCacheMu.Unlock()
	Records.Delete(uuid)
}

func AppendClientReport(uuid string, report v1.Report) (v1.Report, error) {
	reportCacheMu.Lock()
	defer reportCacheMu.Unlock()

	reports, ok := cachedReports(uuid)
	if !ok {
		return v1.Report{}, fmt.Errorf("invalid report type for UUID %s", uuid)
	}
	report.UUID = uuid
	report.UpdatedAt = time.Now()
	reports = append(reports, report)
	Records.Set(uuid, reports, cache.DefaultExpiration)
	return report, nil
}

func SaveClientReportToDB() error {
	return saveClientReportToDB(dbcore.GetDBInstance(), time.Now())
}

func saveClientReportToDB(db *gorm.DB, now time.Time) error {
	saveClientReportMu.Lock()
	defer saveClientReportMu.Unlock()

	lastMinute := now.Add(-time.Minute).Unix()
	var records []models.Record
	var gpuRecords []models.GPURecord
	trafficByRecord := make(map[string]cachedTrafficSummary)

	reportCacheMu.Lock()
	// 先收集所有需要保存的数据，但不修改缓存
	filteredByUUID := make(map[string][]v1.Report)
	for uuid, x := range Records.Items() {
		func() {
			if uuid == "" {
				return
			}

			reports, ok := x.Object.([]v1.Report)
			if !ok {
				log.Printf("Invalid report type for UUID %s", uuid)
				return
			}

			var filtered []v1.Report
			for _, r := range reports {
				if r.UpdatedAt.Unix() >= lastMinute {
					if err := clients.ReportVerify(r); err != nil {
						log.Printf("Invalid report data for UUID %s: %v", uuid, err)
						continue
					}
					filtered = append(filtered, r)
				}
			}

			filteredByUUID[uuid] = filtered

			if len(filtered) > 0 {
				r := utils.AverageReport(uuid, now, filtered, 0.3)
				key := recordDedupKey(r)
				trafficByRecord[key] = summarizeCachedTraffic(filtered)
				records = append(records, r)
				gpuRecords = append(gpuRecords, utils.AverageGPUReports(uuid, now, filtered, 0.3)...)
			}
		}()
	}
	reportCacheMu.Unlock()

	useMetricStore := metricstore.IsEnabled()

	if len(records) > 0 {
		unique := make(map[string]models.Record)
		for _, rec := range records {
			unique[recordDedupKey(rec)] = rec
		}
		var deduped []models.Record
		dedupedTraffic := make(map[string]cachedTrafficSummary, len(unique))
		for key, rec := range unique {
			deduped = append(deduped, rec)
			if summary, ok := trafficByRecord[key]; ok {
				dedupedTraffic[key] = summary
			}
		}
		if useMetricStore {
			// metric store 启用：增量基线来自 metric store，记录写入 metric store
			if err := fillTrafficDeltasFromMetricStore(deduped, dedupedTraffic); err != nil {
				log.Printf("Failed to fill traffic deltas from metric store: %v", err)
				return err
			}
			ctx := context.Background()
			for i := range deduped {
				if err := metricstore.WriteRecord(ctx, deduped[i]); err != nil {
					log.Printf("Failed to write record to metric store: %v", err)
					return err
				}
			}
		} else {
			if err := db.Transaction(func(tx *gorm.DB) error {
				if err := fillTrafficDeltas(tx, deduped, dedupedTraffic); err != nil {
					return err
				}
				if err := tx.Model(&models.Record{}).Create(&deduped).Error; err != nil {
					return err
				}
				return nil
			}); err != nil {
				log.Printf("Failed to save records to database: %v", err)
				return err
			}
		}
	}

	// GPU 数据仅在 metric store 启用时持久化（已移除传统 GPU 表）
	if useMetricStore && len(gpuRecords) > 0 {
		gpuUnique := make(map[string]models.GPURecord)
		for _, rec := range gpuRecords {
			key := rec.Client + "_" + strconv.Itoa(rec.DeviceIndex) + "_" + strconv.FormatInt(rec.Time.ToTime().Unix(), 10)
			gpuUnique[key] = rec
		}
		ctx := context.Background()
		for _, rec := range gpuUnique {
			if err := metricstore.WriteGPURecord(ctx, rec); err != nil {
				log.Printf("Failed to write GPU record to metric store: %v", err)
				return err
			}
		}
	}

	// 数据成功写入数据库后，才清理缓存中已处理的旧数据。
	// 这里重新从当前缓存读取并按时间过滤，保留最近一分钟内的报告
	// （包括写库期间新到达的报告），避免写库失败时丢失尚未持久化的历史数据。
	reportCacheMu.Lock()
	for uuid := range filteredByUUID {
		cached, ok := Records.Get(uuid)
		if !ok || cached == nil {
			continue
		}
		reports, ok := cached.([]v1.Report)
		if !ok {
			continue
		}
		var remaining []v1.Report
		for _, r := range reports {
			if r.UpdatedAt.Unix() >= lastMinute {
				remaining = append(remaining, r)
			}
		}
		Records.Set(uuid, remaining, cache.DefaultExpiration)
	}
	reportCacheMu.Unlock()

	return nil
}

func cachedReports(uuid string) ([]v1.Report, bool) {
	cached, ok := Records.Get(uuid)
	if !ok || cached == nil {
		return []v1.Report{}, true
	}
	reports, ok := cached.([]v1.Report)
	return reports, ok
}

func recordDedupKey(rec models.Record) string {
	return rec.Client + "_" + strconv.FormatInt(rec.Time.ToTime().Unix(), 10)
}

type trafficTotalPoint struct {
	Time      time.Time
	TotalUp   int64
	TotalDown int64
	UpSpeed   int64
	DownSpeed int64
}

type cachedTrafficSummary struct {
	Points []trafficTotalPoint
}

func summarizeCachedTraffic(reports []v1.Report) cachedTrafficSummary {
	points := make([]trafficTotalPoint, 0, len(reports))
	for _, report := range reports {
		points = append(points, trafficTotalPoint{
			Time:      report.UpdatedAt,
			TotalUp:   report.Network.TotalUp,
			TotalDown: report.Network.TotalDown,
			UpSpeed:   report.Network.Up,
			DownSpeed: report.Network.Down,
		})
	}
	sort.SliceStable(points, func(i, j int) bool {
		return points[i].Time.Before(points[j].Time)
	})
	return cachedTrafficSummary{Points: points}
}

type previousTrafficRecord struct {
	Client       string           `gorm:"column:client"`
	Time         models.LocalTime `gorm:"column:time"`
	NetTotalUp   int64            `gorm:"column:net_total_up"`
	NetTotalDown int64            `gorm:"column:net_total_down"`
}

func fillTrafficDeltas(db *gorm.DB, records []models.Record, trafficByRecord map[string]cachedTrafficSummary) error {
	recordsByTime := make(map[time.Time][]int)
	for i := range records {
		before := records[i].Time.ToTime().Round(0)
		recordsByTime[before] = append(recordsByTime[before], i)
	}

	for before, indexes := range recordsByTime {
		clientUUIDs := make([]string, 0, len(indexes))
		seen := make(map[string]struct{}, len(indexes))
		for _, index := range indexes {
			clientUUID := records[index].Client
			if clientUUID == "" {
				continue
			}
			if _, exists := seen[clientUUID]; exists {
				continue
			}
			seen[clientUUID] = struct{}{}
			clientUUIDs = append(clientUUIDs, clientUUID)
		}

		previousByClient, err := getLatestTrafficRecordsBefore(db, clientUUIDs, before)
		if err != nil {
			return fmt.Errorf("load previous traffic records before %s: %w", before.Format(time.RFC3339), err)
		}

		for _, index := range indexes {
			key := recordDedupKey(records[index])
			if summary, ok := trafficByRecord[key]; ok && len(summary.Points) > 0 {
				if previous, exists := previousByClient[records[index].Client]; exists {
					records[index].TrafficUp, records[index].TrafficDown = sumCachedTrafficDeltas(summary, &previous)
				} else {
					records[index].TrafficUp, records[index].TrafficDown = sumCachedTrafficDeltas(summary, nil)
				}
				continue
			}

			previous, exists := previousByClient[records[index].Client]
			if !exists {
				continue
			}
			records[index].TrafficUp = utils.ComputeTrafficDelta(records[index].NetTotalUp, previous.NetTotalUp)
			records[index].TrafficDown = utils.ComputeTrafficDelta(records[index].NetTotalDown, previous.NetTotalDown)
		}
	}

	return nil
}

// fillTrafficDeltasFromMetricStore 在 metric store 启用时计算流量增量，
// 基线（上一周期累计流量）从 metric store 查询。
func fillTrafficDeltasFromMetricStore(records []models.Record, trafficByRecord map[string]cachedTrafficSummary) error {
	ctx := context.Background()
	recordsByTime := make(map[time.Time][]int)
	for i := range records {
		before := records[i].Time.ToTime().Round(0)
		recordsByTime[before] = append(recordsByTime[before], i)
	}

	for before, indexes := range recordsByTime {
		clientUUIDs := make([]string, 0, len(indexes))
		seen := make(map[string]struct{}, len(indexes))
		for _, index := range indexes {
			clientUUID := records[index].Client
			if clientUUID == "" {
				continue
			}
			if _, exists := seen[clientUUID]; exists {
				continue
			}
			seen[clientUUID] = struct{}{}
			clientUUIDs = append(clientUUIDs, clientUUID)
		}

		baseline, err := metricstore.GetLatestTrafficBefore(ctx, clientUUIDs, before)
		if err != nil {
			return fmt.Errorf("load previous traffic from metric store before %s: %w", before.Format(time.RFC3339), err)
		}

		for _, index := range indexes {
			var prev *previousTrafficRecord
			if base, ok := baseline[records[index].Client]; ok {
				prev = &previousTrafficRecord{
					Client:       base.Client,
					Time:         base.Time,
					NetTotalUp:   base.NetTotalUp,
					NetTotalDown: base.NetTotalDown,
				}
			}

			key := recordDedupKey(records[index])
			if summary, ok := trafficByRecord[key]; ok && len(summary.Points) > 0 {
				records[index].TrafficUp, records[index].TrafficDown = sumCachedTrafficDeltas(summary, prev)
				continue
			}

			if prev == nil {
				continue
			}
			records[index].TrafficUp = utils.ComputeTrafficDelta(records[index].NetTotalUp, prev.NetTotalUp)
			records[index].TrafficDown = utils.ComputeTrafficDelta(records[index].NetTotalDown, prev.NetTotalDown)
		}
	}

	return nil
}

func sumCachedTrafficDeltas(summary cachedTrafficSummary, previous *previousTrafficRecord) (int64, int64) {
	if len(summary.Points) == 0 {
		return 0, 0
	}

	startIndex := 0
	var previousUp int64
	var previousDown int64
	var previousTime time.Time
	if previous != nil {
		previousUp = previous.NetTotalUp
		previousDown = previous.NetTotalDown
		previousTime = previous.Time.ToTime()
	} else {
		previousUp = summary.Points[0].TotalUp
		previousDown = summary.Points[0].TotalDown
		previousTime = summary.Points[0].Time
		startIndex = 1
	}

	totalUp := sumPlausibleCachedTrafficDeltas(
		summary.Points,
		startIndex,
		previousUp,
		previousTime,
		func(point trafficTotalPoint) int64 { return point.TotalUp },
		func(point trafficTotalPoint) int64 { return point.UpSpeed },
	)
	totalDown := sumPlausibleCachedTrafficDeltas(
		summary.Points,
		startIndex,
		previousDown,
		previousTime,
		func(point trafficTotalPoint) int64 { return point.TotalDown },
		func(point trafficTotalPoint) int64 { return point.DownSpeed },
	)
	return totalUp, totalDown
}

const (
	cachedTrafficJumpAllowance = int64(256 << 20)
	cachedTrafficSpeedMargin   = 8.0
)

func sumPlausibleCachedTrafficDeltas(
	points []trafficTotalPoint,
	startIndex int,
	previousTotal int64,
	previousTime time.Time,
	totalOf func(trafficTotalPoint) int64,
	speedOf func(trafficTotalPoint) int64,
) int64 {
	var total int64
	for index := startIndex; index < len(points); index++ {
		point := points[index]
		if !point.Time.After(previousTime) {
			continue
		}

		current := totalOf(point)
		delta := utils.ComputeTrafficDelta(current, previousTotal)
		if plausibleCachedTrafficDelta(delta, speedOf(point), point.Time.Sub(previousTime)) {
			total += delta
			previousTotal = current
			previousTime = point.Time
			continue
		}

		// A large cumulative counter that immediately falls back is a stale
		// report delivered after a reconnect. Ignore it without moving the
		// baseline so that the following current sample is still counted.
		if current >= previousTotal && index+1 < len(points) && totalOf(points[index+1]) < current {
			continue
		}

		// A persistent discontinuity usually means an interface set change.
		// Rebase without charging its historical bytes as new traffic.
		previousTotal = current
		previousTime = point.Time
	}
	return total
}

func plausibleCachedTrafficDelta(delta, reportedSpeed int64, elapsed time.Duration) bool {
	if delta <= cachedTrafficJumpAllowance {
		return true
	}
	if reportedSpeed < 0 {
		reportedSpeed = 0
	}
	seconds := elapsed.Seconds()
	if seconds < 1 {
		seconds = 1
	}
	limit := float64(cachedTrafficJumpAllowance) +
		float64(reportedSpeed)*seconds*cachedTrafficSpeedMargin
	return float64(delta) <= limit
}

func getLatestTrafficRecordsBefore(db *gorm.DB, clientUUIDs []string, before time.Time) (map[string]previousTrafficRecord, error) {
	previousByClient := make(map[string]previousTrafficRecord, len(clientUUIDs))
	if len(clientUUIDs) == 0 {
		return previousByClient, nil
	}

	for _, table := range []string{"records", "records_long_term"} {
		records, err := latestTrafficRecordsBeforeFromTable(db, table, clientUUIDs, before)
		if err != nil {
			return nil, err
		}
		for _, record := range records {
			previous, exists := previousByClient[record.Client]
			if !exists || record.Time.ToTime().After(previous.Time.ToTime()) {
				previousByClient[record.Client] = record
			}
		}
	}
	return previousByClient, nil
}

func latestTrafficRecordsBeforeFromTable(db *gorm.DB, table string, clientUUIDs []string, before time.Time) ([]previousTrafficRecord, error) {
	var records []previousTrafficRecord
	latestPerClient := db.Table(table).
		Select("client, MAX(time) AS time").
		Where("client IN ? AND time < ?", clientUUIDs, models.FromTime(before)).
		Group("client")

	err := db.Table(table+" AS r").
		Select("r.client, r.time, r.net_total_up, r.net_total_down").
		Joins("JOIN (?) AS latest ON latest.client = r.client AND latest.time = r.time", latestPerClient).
		Find(&records).Error
	return records, err
}
