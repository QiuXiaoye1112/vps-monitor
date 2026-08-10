package records

import (
	"encoding/csv"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/monitor-monitor/monitor/database/models"
)

var uuid = "7901508c-304f-49aa-b84f-957c33ae6f8a"

var _ = func() bool {
	// 确保 Test 环境中使用 sqlite 内存数据库
	return true
}()

// TestCompactRecord tests the database compaction logic by inserting 4h30m of data (one record per minute),
// then running migrateOldRecords and verifying the aggregation and cleanup.
func TestCompactRecord(t *testing.T) {
	const totalMinutes = 12*60 + 30
	loc := models.GetAppLocation()
	now := time.Date(2026, 6, 15, 14, 30, 0, 0, loc)
	threshold := compactRecordCutoff(now)
	overlapCutoff := threshold.Add(-1 * time.Hour)

	// 使用 sqlite 内存数据库并迁移表结构
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	assert.NoError(t, db.AutoMigrate(&models.Record{}))
	assert.NoError(t, db.Table("records_long_term").AutoMigrate(&models.Record{}))

	expectedGroups := make(map[time.Time]struct{})
	expectedRawRemain := 0

	// 插入数据
	for i := 0; i < totalMinutes; i++ {
		recTime := now.Add(-time.Duration(i) * time.Minute)
		rec := models.Record{Client: uuid, Time: models.FromTime(recTime), Cpu: float32(i), Load: float32(i), Temp: float32(i), Ram: int64(i)}
		err := db.Create(&rec).Error
		assert.NoError(t, err)

		if recTime.Before(threshold) {
			slot := recTime.Truncate(15 * time.Minute)
			expectedGroups[slot] = struct{}{}
		}
		if !recTime.Before(overlapCutoff) {
			expectedRawRemain++
		}
	}

	// 导出原始数据到 CSV
	os.MkdirAll("../../data", 0755)
	var origRecs []models.Record
	db.Order("time desc").Find(&origRecs)
	fOrig, err := os.Create("../../data/original.csv")
	assert.NoError(t, err)
	defer fOrig.Close()
	wOrig := csv.NewWriter(fOrig)
	defer wOrig.Flush()
	wOrig.Write([]string{"Client", "Time", "Cpu", "Load", "Temp", "Ram"})
	for _, r := range origRecs {
		wOrig.Write([]string{
			r.Client,
			r.Time.ToTime().Format(time.RFC3339),
			strconv.FormatFloat(float64(r.Cpu), 'f', -1, 32),
			strconv.FormatFloat(float64(r.Load), 'f', -1, 32),
			strconv.FormatFloat(float64(r.Temp), 'f', -1, 32),
			strconv.FormatInt(r.Ram, 10),
		})
	}

	// 运行压缩（迁移）逻辑
	err = migrateOldRecordsAt(db, now)
	assert.NoError(t, err)

	// 验证 long-term 表中的聚合记录数
	var longCount int64
	assert.NoError(t, db.Table("records_long_term").Count(&longCount).Error)
	assert.Equal(t, int64(len(expectedGroups)), longCount)

	// 验证原始表中剩余记录数
	var remainCount int64
	assert.NoError(t, db.Table("records").Count(&remainCount).Error)
	assert.Equal(t, int64(expectedRawRemain), remainCount)

	// 导出压缩后的数据到 CSV
	var compRecs []models.Record
	db.Table("records_long_term").Order("time desc").Find(&compRecs)
	fComp, err := os.Create("../../data/compressed.csv")
	assert.NoError(t, err)
	defer fComp.Close()
	wComp := csv.NewWriter(fComp)
	defer wComp.Flush()
	wComp.Write([]string{"Client", "Time", "Cpu", "Load", "Temp", "Ram"})
	for _, r := range compRecs {
		wComp.Write([]string{
			r.Client,
			r.Time.ToTime().Format(time.RFC3339),
			strconv.FormatFloat(float64(r.Cpu), 'f', -1, 32),
			strconv.FormatFloat(float64(r.Load), 'f', -1, 32),
			strconv.FormatFloat(float64(r.Temp), 'f', -1, 32),
			strconv.FormatInt(r.Ram, 10),
		})
	}

	db.Table("records").Order("time desc").Find(&compRecs)
	fComp, err = os.Create("../../data/compressed_records.csv")
	assert.NoError(t, err)
	defer fComp.Close()
	wComp = csv.NewWriter(fComp)
	defer wComp.Flush()
	wComp.Write([]string{"Client", "Time", "Cpu", "Load", "Temp", "Ram"})
	for _, r := range compRecs {
		wComp.Write([]string{
			r.Client,
			r.Time.ToTime().Format(time.RFC3339),
			strconv.FormatFloat(float64(r.Cpu), 'f', -1, 32),
			strconv.FormatFloat(float64(r.Load), 'f', -1, 32),
			strconv.FormatFloat(float64(r.Temp), 'f', -1, 32),
			strconv.FormatInt(r.Ram, 10),
		})
	}
}

func TestCompactRecordPreservesExactTrafficDelta(t *testing.T) {
	loc := models.GetAppLocation()
	currentTime := time.Date(2026, 6, 15, 14, 30, 0, 0, loc)
	now := currentTime.Truncate(15 * time.Minute).Add(-5*time.Hour + time.Minute)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	assert.NoError(t, db.AutoMigrate(&models.Record{}))
	assert.NoError(t, db.Table("records_long_term").AutoMigrate(&models.Record{}))

	records := []models.Record{
		{
			Client:       uuid,
			Time:         models.FromTime(now),
			NetTotalUp:   100,
			NetTotalDown: 200,
			TrafficUp:    0,
			TrafficDown:  0,
		},
		{
			Client:       uuid,
			Time:         models.FromTime(now.Add(5 * time.Minute)),
			NetTotalUp:   150,
			NetTotalDown: 260,
			TrafficUp:    50,
			TrafficDown:  60,
		},
		{
			Client:       uuid,
			Time:         models.FromTime(now.Add(10 * time.Minute)),
			NetTotalUp:   10,
			NetTotalDown: 30,
			TrafficUp:    10,
			TrafficDown:  30,
		},
	}

	for _, rec := range records {
		assert.NoError(t, db.Create(&rec).Error)
	}

	assert.NoError(t, migrateOldRecordsAt(db, currentTime))

	var compacted []models.Record
	assert.NoError(t, db.Table("records_long_term").Find(&compacted).Error)
	require.Len(t, compacted, 1)
	assert.Equal(t, int64(60), compacted[0].TrafficUp)
	assert.Equal(t, int64(90), compacted[0].TrafficDown)
	assert.Equal(t, int64(10), compacted[0].NetTotalUp)
	assert.Equal(t, int64(30), compacted[0].NetTotalDown)
	assert.True(t, compacted[0].Time.ToTime().Equal(records[2].Time.ToTime().Truncate(15*time.Minute)))
}

func TestManualTrafficClearSurvivesCompactionAndHistoryCleanup(t *testing.T) {
	loc := models.GetAppLocation()
	slot := time.Date(2026, 6, 15, 10, 0, 0, 0, loc)
	clearedAt := slot.Add(7*time.Minute + 30*time.Second)
	compactionNow := slot.Add(4*time.Hour + 30*time.Minute)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Client{}, &models.Record{}))
	require.NoError(t, db.Table("records_long_term").AutoMigrate(&models.Record{}))

	client := models.Client{
		UUID:             "manual-clear-node",
		Token:            "manual-clear-token",
		TrafficClearedAt: models.FromTime(clearedAt),
	}
	require.NoError(t, db.Create(&client).Error)
	require.NoError(t, db.Model(&models.Client{}).Where("uuid = ?", client.UUID).
		Update("traffic_reset_enabled", false).Error)

	for _, record := range []models.Record{
		{
			Client: client.UUID, Time: models.FromTime(slot.Add(5 * time.Minute)),
			TrafficUp: 60, TrafficDown: 40,
		},
		{
			Client: client.UUID, Time: models.FromTime(slot.Add(8 * time.Minute)),
			TrafficUp: 12, TrafficDown: 8,
		},
		{
			Client: client.UUID, Time: models.FromTime(slot.Add(14 * time.Minute)),
			TrafficUp: 18, TrafficDown: 12,
		},
	} {
		require.NoError(t, db.Create(&record).Error)
	}

	require.NoError(t, migrateOldRecordsAt(db, compactionNow))
	// The raw overlap remains for an hour. Re-running compaction during that
	// overlap must update the split rows rather than double-counting them.
	require.NoError(t, migrateOldRecordsAt(db, compactionNow))

	var compacted []models.Record
	require.NoError(t, db.Table("records_long_term").Order("time ASC").Find(&compacted).Error)
	require.Len(t, compacted, 2)
	require.True(t, compacted[0].Time.ToTime().Equal(slot))
	require.Equal(t, int64(60), compacted[0].TrafficUp)
	require.Equal(t, int64(40), compacted[0].TrafficDown)
	require.True(t, compacted[1].Time.ToTime().Equal(clearedAt))
	require.Equal(t, int64(30), compacted[1].TrafficUp)
	require.Equal(t, int64(20), compacted[1].TrafficDown)

	up, down, err := sumLegacyTrafficDeltas(db, client.UUID, clearedAt, compactionNow)
	require.NoError(t, err)
	require.Equal(t, int64(30), up)
	require.Equal(t, int64(20), down)

	historyCutoff := slot.Add(24 * time.Hour)
	require.NoError(t, deleteLegacyRecordsBefore(db, historyCutoff, historyCutoff))

	var updated models.Client
	require.NoError(t, db.Where("uuid = ?", client.UUID).First(&updated).Error)
	require.Equal(t, int64(30), updated.TrafficCarryUp)
	require.Equal(t, int64(20), updated.TrafficCarryDown)

	up, down, err = sumLegacyTrafficDeltas(db, client.UUID, clearedAt, historyCutoff)
	require.NoError(t, err)
	require.Zero(t, up)
	require.Zero(t, down)
	require.Equal(t, int64(50), up+down+updated.TrafficCarryUp+updated.TrafficCarryDown)
}

func TestCompactionRepairsZeroDeltaFromStrictClearBaseline(t *testing.T) {
	clearedAt := time.Date(2026, 7, 28, 12, 7, 30, 0, time.UTC)
	records := []models.Record{{
		Client:       "strict-clear-repair",
		Time:         models.FromTime(clearedAt.Add(time.Minute)),
		NetTotalUp:   1_025,
		NetTotalDown: 2_040,
	}}
	previous := map[string]*models.Record{
		"strict-clear-repair": {
			Client:       "strict-clear-repair",
			Time:         models.FromTime(clearedAt.Add(-time.Minute)),
			NetTotalUp:   900,
			NetTotalDown: 1_900,
		},
	}
	baselines := map[string]trafficClearBaseline{
		"strict-clear-repair": {
			At:   clearedAt,
			Up:   1_000,
			Down: 2_000,
		},
	}

	repairZeroTrafficDeltasWithBaselines(records, previous, baselines)
	require.Equal(t, int64(25), records[0].TrafficUp)
	require.Equal(t, int64(40), records[0].TrafficDown)
}

func TestRepairZeroTrafficDeltasPreservesRawResetDetailBeforeCompaction(t *testing.T) {
	loc := models.GetAppLocation()
	start := time.Date(2026, 6, 6, 0, 0, 0, 0, loc)
	records := []models.Record{
		{Client: uuid, Time: models.FromTime(start), NetTotalUp: 100, NetTotalDown: 200},
		{Client: uuid, Time: models.FromTime(start.Add(5 * time.Minute)), NetTotalUp: 140, NetTotalDown: 260},
		{Client: uuid, Time: models.FromTime(start.Add(10 * time.Minute)), NetTotalUp: 10, NetTotalDown: 20},
		{Client: uuid, Time: models.FromTime(start.Add(15 * time.Minute)), NetTotalUp: 25, NetTotalDown: 35},
	}

	repairZeroTrafficDeltas(records, nil)

	assert.Equal(t, int64(0), records[0].TrafficUp)
	assert.Equal(t, int64(0), records[0].TrafficDown)
	assert.Equal(t, int64(40), records[1].TrafficUp)
	assert.Equal(t, int64(60), records[1].TrafficDown)
	assert.Equal(t, int64(10), records[2].TrafficUp)
	assert.Equal(t, int64(20), records[2].TrafficDown)
	assert.Equal(t, int64(15), records[3].TrafficUp)
	assert.Equal(t, int64(15), records[3].TrafficDown)
}

func TestRepairZeroTrafficDeltasUsesPreviousPersistedBaseline(t *testing.T) {
	loc := models.GetAppLocation()
	now := time.Date(2026, 6, 7, 12, 0, 0, 0, loc)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	assert.NoError(t, db.AutoMigrate(&models.Record{}))
	assert.NoError(t, db.Table("records_long_term").AutoMigrate(&models.Record{}))

	assert.NoError(t, db.Table("records_long_term").Create(&models.Record{
		Client:       uuid,
		Time:         models.FromTime(now.Add(-10 * time.Minute)),
		NetTotalUp:   100,
		NetTotalDown: 200,
	}).Error)

	rawRecords := []models.Record{
		{Client: uuid, Time: models.FromTime(now), NetTotalUp: 150, NetTotalDown: 260},
		{Client: uuid, Time: models.FromTime(now.Add(5 * time.Minute)), NetTotalUp: 175, NetTotalDown: 300},
	}

	previousByClient, err := getPreviousTrafficRecordsBefore(db, rawRecords)
	assert.NoError(t, err)
	repairZeroTrafficDeltas(rawRecords, previousByClient)

	assert.Equal(t, int64(50), rawRecords[0].TrafficUp)
	assert.Equal(t, int64(60), rawRecords[0].TrafficDown)
	assert.Equal(t, int64(25), rawRecords[1].TrafficUp)
	assert.Equal(t, int64(40), rawRecords[1].TrafficDown)
}

func TestRepairZeroTrafficDeltasIgnoresSameSlotLongTermBaseline(t *testing.T) {
	loc := models.GetAppLocation()
	now := time.Date(2026, 6, 7, 12, 0, 30, 0, loc)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	assert.NoError(t, db.AutoMigrate(&models.Record{}))
	assert.NoError(t, db.Table("records_long_term").AutoMigrate(&models.Record{}))

	assert.NoError(t, db.Table("records_long_term").Create(&models.Record{
		Client:       uuid,
		Time:         models.FromTime(now.Truncate(15 * time.Minute)),
		NetTotalUp:   900,
		NetTotalDown: 900,
	}).Error)

	rawRecords := []models.Record{
		{Client: uuid, Time: models.FromTime(now), NetTotalUp: 150, NetTotalDown: 260},
		{Client: uuid, Time: models.FromTime(now.Add(5 * time.Minute)), NetTotalUp: 175, NetTotalDown: 300},
	}

	previousByClient, err := getPreviousTrafficRecordsBefore(db, rawRecords)
	assert.NoError(t, err)
	repairZeroTrafficDeltas(rawRecords, previousByClient)

	assert.Equal(t, int64(0), rawRecords[0].TrafficUp)
	assert.Equal(t, int64(0), rawRecords[0].TrafficDown)
	assert.Equal(t, int64(25), rawRecords[1].TrafficUp)
	assert.Equal(t, int64(40), rawRecords[1].TrafficDown)
}

func TestCompactRecordRetainsOneHourOverlapWindow(t *testing.T) {
	loc := models.GetAppLocation()
	now := time.Date(2026, 6, 7, 12, 7, 0, 0, loc)
	cutoff := compactRecordCutoff(now)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	assert.NoError(t, db.AutoMigrate(&models.Record{}))
	assert.NoError(t, db.Table("records_long_term").AutoMigrate(&models.Record{}))

	records := []models.Record{
		{Client: uuid, Time: models.FromTime(cutoff.Add(-time.Hour - time.Minute)), TrafficUp: 5},
		{Client: uuid, Time: models.FromTime(cutoff.Add(-30 * time.Minute)), TrafficUp: 7},
		{Client: uuid, Time: models.FromTime(now.Add(-3 * time.Hour)), TrafficUp: 11},
	}
	for _, rec := range records {
		assert.NoError(t, db.Create(&rec).Error)
	}

	assert.NoError(t, migrateOldRecordsAt(db, now))

	var remainTimes []models.Record
	assert.NoError(t, db.Table("records").Order("time ASC").Find(&remainTimes).Error)
	require.Len(t, remainTimes, 2)
	assert.True(t, remainTimes[0].Time.ToTime().Equal(records[1].Time.ToTime()))
	assert.True(t, remainTimes[1].Time.ToTime().Equal(records[2].Time.ToTime()))
}

func TestCompactRecordOnlyMigratesCompleteFifteenMinuteBuckets(t *testing.T) {
	loc := models.GetAppLocation()
	now := time.Date(2026, 6, 7, 12, 7, 0, 0, loc)
	cutoff := compactRecordCutoff(now)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	assert.NoError(t, db.AutoMigrate(&models.Record{}))
	assert.NoError(t, db.Table("records_long_term").AutoMigrate(&models.Record{}))

	compactable := models.Record{Client: uuid, Time: models.FromTime(cutoff.Add(-time.Minute)), TrafficUp: 5}
	partialSlot := models.Record{Client: uuid, Time: models.FromTime(cutoff.Add(time.Minute)), TrafficUp: 7}
	assert.NoError(t, db.Create(&compactable).Error)
	assert.NoError(t, db.Create(&partialSlot).Error)

	assert.NoError(t, migrateOldRecordsAt(db, now))

	var compacted []models.Record
	assert.NoError(t, db.Table("records_long_term").Order("time ASC").Find(&compacted).Error)
	require.Len(t, compacted, 1)
	assert.True(t, compacted[0].Time.ToTime().Equal(compactable.Time.ToTime().Truncate(15*time.Minute)))

	var rawCount int64
	assert.NoError(t, db.Table("records").Where("time = ?", partialSlot.Time).Count(&rawCount).Error)
	assert.Equal(t, int64(1), rawCount)
}

func TestTrafficWindowEdgeCases(t *testing.T) {
	loc := trafficLocation()
	client := models.Client{
		TrafficResetDay:     31,
		TrafficResetHour:    0,
		TrafficResetEnabled: true,
	}

	// Case 1: March 30, 2026.
	// Expected: Cycle starts Feb 28, ends March 31.
	now1 := time.Date(2026, 3, 30, 12, 0, 0, 0, loc)
	start1, end1 := TrafficWindow(client, now1)
	assert.True(t, start1.Equal(time.Date(2026, 2, 28, 0, 0, 0, 0, loc)), "start1 was %v", start1)
	assert.True(t, end1.Equal(time.Date(2026, 3, 31, 0, 0, 0, 0, loc)), "end1 was %v", end1)

	// Case 2: March 31, 2026.
	// Expected: Cycle starts March 31, ends April 30.
	now2 := time.Date(2026, 3, 31, 12, 0, 0, 0, loc)
	start2, end2 := TrafficWindow(client, now2)
	assert.True(t, start2.Equal(time.Date(2026, 3, 31, 0, 0, 0, 0, loc)), "start2 was %v", start2)
	assert.True(t, end2.Equal(time.Date(2026, 4, 30, 0, 0, 0, 0, loc)), "end2 was %v", end2)
}

func TestApplyLocalTimeRangeUsesStoredTimeFormat(t *testing.T) {
	db := newTrafficTestDB(t)
	client := "records-range-node"
	appLoc := models.GetAppLocation()
	reset := time.Date(2026, 8, 10, 3, 45, 0, 0, time.UTC)
	end := time.Date(2026, 8, 10, 5, 7, 37, 0, time.UTC)
	storageTime := func(instant time.Time) string {
		return instant.In(appLoc).Format("2006-01-02 15:04:05.0000000")
	}

	for _, row := range []struct {
		stamp string
		value int
	}{
		{storageTime(reset.Add(-time.Minute)), 1},
		{storageTime(reset), 2},
		{storageTime(time.Date(2026, 8, 10, 5, 0, 0, 0, time.UTC)), 3},
	} {
		require.NoError(t, db.Exec("INSERT INTO records (client, time, process) VALUES (?, ?, ?)",
			client, row.stamp, row.value).Error)
	}

	var records []models.Record
	require.NoError(t, applyLocalTimeRange(db.Table("records"), reset, end).
		Order("time ASC").Find(&records).Error)
	require.Len(t, records, 2)
	require.Equal(t, 2, records[0].Process)
	require.Equal(t, 3, records[1].Process)
}

func TestTrafficResetSwitch(t *testing.T) {
	loc := trafficLocation()

	// 测试 1: 开关开启
	cEnabled := models.Client{
		TrafficResetDay:     1,
		TrafficResetHour:    0,
		TrafficResetEnabled: true,
	}
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, loc)
	start, end := TrafficWindow(cEnabled, now)
	assert.True(t, start.Equal(time.Date(2026, 7, 1, 0, 0, 0, 0, loc)))
	assert.True(t, end.Equal(time.Date(2026, 8, 1, 0, 0, 0, 0, loc)))

	// 测试 2: 开关关闭（不应重置，窗口需为大区间）
	cDisabled := models.Client{
		TrafficResetDay:     1,
		TrafficResetHour:    0,
		TrafficResetEnabled: false,
	}
	startD, endD := TrafficWindow(cDisabled, now)
	assert.True(t, startD.IsZero(), "Start window should be zero when reset is disabled")
	assert.True(t, endD.After(now.AddDate(50, 0, 0)), "End window should be in the far future")
}
