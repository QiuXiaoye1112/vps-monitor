package records

import (
	"errors"
	"time"

	"github.com/monitor-monitor/monitor/database/dbcore"
	"github.com/monitor-monitor/monitor/database/models"
	"gorm.io/gorm"
)

type MonthlyTraffic struct {
	Start        time.Time `json:"start"`
	End          time.Time `json:"end"`
	Up           int64     `json:"up"`
	Down         int64     `json:"down"`
	RawTotal     int64     `json:"raw_total"`
	CarryUp      int64     `json:"-"`
	CarryDown    int64     `json:"-"`
	Compensation int64     `json:"compensation"`
	Total        int64     `json:"total"`
}

func CurrentMonthlyTraffic(client models.Client, now time.Time) (MonthlyTraffic, error) {
	windowStart, end := TrafficWindow(client, now)
	start := trafficAccountingStart(client, windowStart)
	up, down, err := sumTrafficDeltas(client.UUID, start, now)
	if err != nil {
		return MonthlyTraffic{}, err
	}

	compensation, carryUp, carryDown := trafficAdjustmentsForWindow(client, windowStart)
	raw := up + down + carryUp + carryDown
	total := raw + compensation
	if total < 0 {
		total = 0
	}

	return MonthlyTraffic{
		Start:        start,
		End:          end,
		Up:           up,
		Down:         down,
		RawTotal:     raw,
		CarryUp:      carryUp,
		CarryDown:    carryDown,
		Compensation: compensation,
		Total:        total,
	}, nil
}

// trafficAdjustmentsForWindow makes the configured reset boundary exact from
// the reader's perspective. The minute scheduler still persists zeroes, but
// stale compensation/carry from the previous window can never leak into the
// new window while that scheduled write is pending.
func trafficAdjustmentsForWindow(client models.Client, windowStart time.Time) (compensation, carryUp, carryDown int64) {
	compensation = client.TrafficComp
	carryUp = client.TrafficCarryUp
	carryDown = client.TrafficCarryDown
	if !client.TrafficResetEnabled || windowStart.IsZero() {
		return
	}

	resetAt := client.TrafficCompResetAt.ToTime()
	if resetAt.IsZero() {
		resetAt = client.CreatedAt.ToTime()
	}
	if resetAt.Before(windowStart) {
		return 0, 0, 0
	}
	return
}

func trafficAccountingStart(client models.Client, windowStart time.Time) time.Time {
	clearedAt := client.TrafficClearedAt.ToTime()
	if clearedAt.After(windowStart) {
		return clearedAt
	}
	return windowStart
}

func TrafficWindow(client models.Client, now time.Time) (time.Time, time.Time) {
	if !client.TrafficResetEnabled {
		return time.Time{}, now.AddDate(100, 0, 0)
	}
	loc := trafficLocation()
	localNow := now.In(loc)
	day := client.TrafficResetDay
	if day <= 0 {
		day = 1
	}
	hour := client.TrafficResetHour
	if hour < 0 || hour > 23 {
		hour = 0
	}
	minute := client.TrafficResetMinute
	if minute < 0 || minute > 59 {
		minute = 0
	}

	this := monthlyBoundary(localNow.Year(), localNow.Month(), day, hour, minute, loc)
	var start time.Time
	var end time.Time
	if localNow.Before(this) {
		prevYear := localNow.Year()
		prevMonth := localNow.Month() - 1
		if prevMonth == 0 {
			prevMonth = 12
			prevYear--
		}
		start = monthlyBoundary(prevYear, prevMonth, day, hour, minute, loc)
		end = this
	} else {
		start = this
		nextYear := localNow.Year()
		nextMonth := localNow.Month() + 1
		if nextMonth == 13 {
			nextMonth = 1
			nextYear++
		}
		end = monthlyBoundary(nextYear, nextMonth, day, hour, minute, loc)
	}
	return start, end
}

func monthlyBoundary(year int, month time.Month, day, hour, minute int, loc *time.Location) time.Time {
	lastDay := time.Date(year, month+1, 0, hour, minute, 0, 0, loc).Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(year, month, day, hour, minute, 0, 0, loc)
}

func trafficLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	return loc
}

func sumTrafficDeltas(uuid string, start, end time.Time) (int64, int64, error) {
	return sumLegacyTrafficDeltas(dbcore.GetDBInstance(), uuid, start, end)
}

// applyLocalTimeRange converts Go time values to the same timezone-less string
// representation used by models.LocalTime.Value before binding them to
// SQLite. A zero bound means that side of the range is unbounded.
func applyLocalTimeRange(query *gorm.DB, start, end time.Time) *gorm.DB {
	if !start.IsZero() {
		query = query.Where("time >= ?", models.FromTime(start))
	}
	if !end.IsZero() {
		query = query.Where("time <= ?", models.FromTime(end))
	}
	return query
}

func applyTrafficTimeRange(query *gorm.DB, uuid string, start, end time.Time) *gorm.DB {
	return applyLocalTimeRange(query.Where("client = ?", uuid), start, end)
}

// sumLegacyTrafficDeltas joins the compacted and raw record streams at the
// actual last compacted slot. A wall-clock cutoff is not safe here: compaction
// runs periodically, so its newest slot can lag the nominal four-hour cutoff
// by several minutes. Using that nominal cutoff creates a moving gap and makes
// an otherwise cumulative traffic total decrease until the next compaction.
func sumLegacyTrafficDeltas(db *gorm.DB, uuid string, start, end time.Time) (int64, int64, error) {
	var recent struct {
		Up   int64
		Down int64
	}
	var archived struct {
		Up   int64
		Down int64
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := applyTrafficTimeRange(tx.Table("records_long_term").
			Select("COALESCE(SUM(CASE WHEN traffic_up > 0 THEN traffic_up ELSE 0 END), 0) AS up, COALESCE(SUM(CASE WHEN traffic_down > 0 THEN traffic_down ELSE 0 END), 0) AS down"), uuid, start, end).
			Scan(&archived).Error; err != nil {
			return err
		}

		recentStart := start
		var latestArchived models.Record
		result := applyTrafficTimeRange(tx.Table("records_long_term").
			Select("time"), uuid, start, end).
			Order("time DESC").
			Limit(1).
			Take(&latestArchived)
		if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return result.Error
		}
		if result.Error == nil {
			afterArchivedSlot := latestArchived.Time.ToTime().
				Truncate(longTermRecordInterval).
				Add(longTermRecordInterval)
			if afterArchivedSlot.After(recentStart) {
				recentStart = afterArchivedSlot
			}
		}

		if recentStart.After(end) {
			return nil
		}
		return applyTrafficTimeRange(tx.Table("records").
			Select("COALESCE(SUM(CASE WHEN traffic_up > 0 THEN traffic_up ELSE 0 END), 0) AS up, COALESCE(SUM(CASE WHEN traffic_down > 0 THEN traffic_down ELSE 0 END), 0) AS down"), uuid, recentStart, end).
			Scan(&recent).Error
	})
	if err != nil {
		return 0, 0, err
	}

	return recent.Up + archived.Up, recent.Down + archived.Down, nil
}
