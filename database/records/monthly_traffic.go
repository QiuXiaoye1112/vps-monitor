package records

import (
	"errors"
	"time"

	"github.com/monitor-monitor/monitor/database/dbcore"
	"github.com/monitor-monitor/monitor/database/metricstore"
	"github.com/monitor-monitor/monitor/database/models"
	"gorm.io/gorm"
)

type MonthlyTraffic struct {
	Start        time.Time `json:"start"`
	End          time.Time `json:"end"`
	Up           int64     `json:"up"`
	Down         int64     `json:"down"`
	RawTotal     int64     `json:"raw_total"`
	Compensation int64     `json:"compensation"`
	Total        int64     `json:"total"`
}

func CurrentMonthlyTraffic(client models.Client, now time.Time) (MonthlyTraffic, error) {
	start, end := TrafficWindow(client, now)
	up, down, err := sumTrafficDeltas(client.UUID, start, now)
	if err != nil {
		return MonthlyTraffic{}, err
	}

	raw := up + down
	total := raw + client.TrafficComp
	if total < 0 {
		total = 0
	}

	return MonthlyTraffic{
		Start:        start,
		End:          end,
		Up:           up,
		Down:         down,
		RawTotal:     raw,
		Compensation: client.TrafficComp,
		Total:        total,
	}, nil
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

	this := monthlyBoundary(localNow.Year(), localNow.Month(), day, hour, loc)
	var start time.Time
	var end time.Time
	if localNow.Before(this) {
		prevYear := localNow.Year()
		prevMonth := localNow.Month() - 1
		if prevMonth == 0 {
			prevMonth = 12
			prevYear--
		}
		start = monthlyBoundary(prevYear, prevMonth, day, hour, loc)
		end = this
	} else {
		start = this
		nextYear := localNow.Year()
		nextMonth := localNow.Month() + 1
		if nextMonth == 13 {
			nextMonth = 1
			nextYear++
		}
		end = monthlyBoundary(nextYear, nextMonth, day, hour, loc)
	}
	return start, end
}

func monthlyBoundary(year int, month time.Month, day, hour int, loc *time.Location) time.Time {
	lastDay := time.Date(year, month+1, 0, hour, 0, 0, 0, loc).Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(year, month, day, hour, 0, 0, 0, loc)
}

func trafficLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	return loc
}

func sumTrafficDeltas(uuid string, start, end time.Time) (int64, int64, error) {
	if metricstore.IsEnabled() {
		recs, err := GetRecordsByClientAndTime(uuid, start, end)
		if err != nil {
			return 0, 0, err
		}
		var up, down int64
		for _, rec := range recs {
			if rec.TrafficUp > 0 {
				up += rec.TrafficUp
			}
			if rec.TrafficDown > 0 {
				down += rec.TrafficDown
			}
		}
		return up, down, nil
	}

	return sumLegacyTrafficDeltas(dbcore.GetDBInstance(), uuid, start, end)
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
		if err := tx.Table("records_long_term").
			Select("COALESCE(SUM(CASE WHEN traffic_up > 0 THEN traffic_up ELSE 0 END), 0) AS up, COALESCE(SUM(CASE WHEN traffic_down > 0 THEN traffic_down ELSE 0 END), 0) AS down").
			Where("client = ? AND time >= ? AND time <= ?", uuid, start, end).
			Scan(&archived).Error; err != nil {
			return err
		}

		recentStart := start
		var latestArchived models.Record
		result := tx.Table("records_long_term").
			Select("time").
			Where("client = ? AND time >= ? AND time <= ?", uuid, start, end).
			Order("time DESC").
			Limit(1).
			Take(&latestArchived)
		if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return result.Error
		}
		if result.Error == nil {
			afterArchivedSlot := latestArchived.Time.ToTime().Add(longTermRecordInterval)
			if afterArchivedSlot.After(recentStart) {
				recentStart = afterArchivedSlot
			}
		}

		if recentStart.After(end) {
			return nil
		}
		return tx.Table("records").
			Select("COALESCE(SUM(CASE WHEN traffic_up > 0 THEN traffic_up ELSE 0 END), 0) AS up, COALESCE(SUM(CASE WHEN traffic_down > 0 THEN traffic_down ELSE 0 END), 0) AS down").
			Where("client = ? AND time >= ? AND time <= ?", uuid, recentStart, end).
			Scan(&recent).Error
	})
	if err != nil {
		return 0, 0, err
	}

	return recent.Up + archived.Up, recent.Down + archived.Down, nil
}
