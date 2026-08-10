package report

import (
	"fmt"
	"sync"
	"time"

	"github.com/monitor-monitor/monitor/database/clients"
	"github.com/monitor-monitor/monitor/database/models"
	"github.com/monitor-monitor/monitor/database/records"
	reportmodel "github.com/monitor-monitor/monitor/protocol/report"
	"github.com/monitor-monitor/monitor/utils"
)

var (
	historyMu      sync.Mutex
	historySaveMu  sync.Mutex
	pendingHistory []models.Record
)

func AppendHistoryReport(uuid string, report reportmodel.Report) error {
	if uuid == "" {
		return fmt.Errorf("invalid client UUID")
	}
	report.UUID = uuid
	report.UpdatedAt = time.Now()
	if err := clients.ReportVerify(report); err != nil {
		return err
	}
	record := utils.AverageReport(uuid, report.UpdatedAt, []reportmodel.Report{report}, 0)

	historyMu.Lock()
	pendingHistory = append(pendingHistory, record)
	historyMu.Unlock()
	return nil
}

func SaveHistoryReportsToDB() error {
	historySaveMu.Lock()
	defer historySaveMu.Unlock()

	historyMu.Lock()
	batch := pendingHistory
	pendingHistory = nil
	historyMu.Unlock()
	if len(batch) == 0 {
		return nil
	}

	if err := records.SaveHistoryRecords(batch); err != nil {
		historyMu.Lock()
		pendingHistory = append(batch, pendingHistory...)
		historyMu.Unlock()
		return err
	}
	return nil
}

func DeleteClientHistoryReports(uuid string) {
	historySaveMu.Lock()
	defer historySaveMu.Unlock()
	historyMu.Lock()
	defer historyMu.Unlock()

	filtered := pendingHistory[:0]
	for _, record := range pendingHistory {
		if record.Client != uuid {
			filtered = append(filtered, record)
		}
	}
	pendingHistory = filtered
}
