package report

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/monitor-monitor/monitor/database/clients"
	"github.com/monitor-monitor/monitor/database/dbcore"
	"github.com/monitor-monitor/monitor/database/models"
	reportmodel "github.com/monitor-monitor/monitor/protocol/report"
	"github.com/monitor-monitor/monitor/utils"
	"github.com/patrickmn/go-cache"
	"gorm.io/gorm"
)

var Records = cache.New(1*time.Minute, 1*time.Minute)
var reportCacheMu sync.Mutex
var saveClientReportMu sync.Mutex

func DeleteClientReports(uuid string) {
	saveClientReportMu.Lock()
	defer saveClientReportMu.Unlock()
	reportCacheMu.Lock()
	defer reportCacheMu.Unlock()
	Records.Delete(uuid)
	DeleteClientHistoryReports(uuid)
}

func AppendClientReport(uuid string, report reportmodel.Report) (reportmodel.Report, error) {
	reportCacheMu.Lock()
	defer reportCacheMu.Unlock()
	reports, ok := cachedReports(uuid)
	if !ok {
		return reportmodel.Report{}, fmt.Errorf("invalid report type for UUID %s", uuid)
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

// saveClientReportToDB stores sampled monitoring history only. Network totals
// are already the Agent's persistent cycle ledger; the center never derives
// traffic deltas, baselines, carry values, or another cumulative total.
func saveClientReportToDB(db *gorm.DB, now time.Time) error {
	saveClientReportMu.Lock()
	defer saveClientReportMu.Unlock()

	lastMinute := now.Add(-time.Minute).Unix()
	var records []models.Record
	filteredByUUID := make(map[string][]reportmodel.Report)

	reportCacheMu.Lock()
	for uuid, item := range Records.Items() {
		if uuid == "" {
			continue
		}
		reports, ok := item.Object.([]reportmodel.Report)
		if !ok {
			log.Printf("Invalid report type for UUID %s", uuid)
			continue
		}
		filtered := make([]reportmodel.Report, 0, len(reports))
		for _, report := range reports {
			if report.UpdatedAt.Unix() < lastMinute {
				continue
			}
			if err := clients.ReportVerify(report); err != nil {
				log.Printf("Invalid report data for UUID %s: %v", uuid, err)
				continue
			}
			filtered = append(filtered, report)
		}
		filteredByUUID[uuid] = filtered
		if len(filtered) > 0 {
			records = append(records, utils.AverageReport(uuid, now, filtered, 0.3))
		}
	}
	reportCacheMu.Unlock()

	if len(records) > 0 {
		unique := make(map[string]models.Record, len(records))
		for _, record := range records {
			key := record.Client + "_" + record.Time.ToTime().Format(time.RFC3339Nano)
			unique[key] = record
		}
		deduped := make([]models.Record, 0, len(unique))
		for _, record := range unique {
			deduped = append(deduped, record)
		}
		if err := db.Model(&models.Record{}).Create(&deduped).Error; err != nil {
			log.Printf("Failed to save records to database: %v", err)
			return err
		}
	}

	reportCacheMu.Lock()
	for uuid := range filteredByUUID {
		cached, ok := Records.Get(uuid)
		if !ok || cached == nil {
			continue
		}
		reports, ok := cached.([]reportmodel.Report)
		if !ok {
			continue
		}
		remaining := make([]reportmodel.Report, 0, len(reports))
		for _, report := range reports {
			if report.UpdatedAt.Unix() >= lastMinute {
				remaining = append(remaining, report)
			}
		}
		Records.Set(uuid, remaining, cache.DefaultExpiration)
	}
	reportCacheMu.Unlock()
	return nil
}

func cachedReports(uuid string) ([]reportmodel.Report, bool) {
	cached, ok := Records.Get(uuid)
	if !ok || cached == nil {
		return []reportmodel.Report{}, true
	}
	reports, ok := cached.([]reportmodel.Report)
	return reports, ok
}
