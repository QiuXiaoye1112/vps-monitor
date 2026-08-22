package report

import (
	"testing"
	"time"

	"github.com/monitor-monitor/monitor/database/models"
	reportmodel "github.com/monitor-monitor/monitor/protocol/report"
	"github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openReportCacheTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Record{}))
	return db
}

func resetReportCache(t *testing.T) {
	t.Helper()
	Records.Flush()
	t.Cleanup(Records.Flush)
}

func TestAppendClientReportRejectsCorruptedCacheValue(t *testing.T) {
	resetReportCache(t)
	Records.Set("client-bad-cache", "not reports", cache.DefaultExpiration)
	_, err := AppendClientReport("client-bad-cache", reportmodel.Report{})
	require.Error(t, err)
}

func TestSaveClientReportStoresAgentCycleTotalsWithoutCenterDelta(t *testing.T) {
	resetReportCache(t)
	db := openReportCacheTestDB(t)
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	Records.Set("node", []reportmodel.Report{
		{UpdatedAt: now.Add(-30 * time.Second), Network: reportmodel.NetworkReport{TotalUp: 100, TotalDown: 200}},
		{UpdatedAt: now.Add(-5 * time.Second), Network: reportmodel.NetworkReport{TotalUp: 175, TotalDown: 320}},
	}, cache.DefaultExpiration)

	require.NoError(t, saveClientReportToDB(db, now))
	var saved models.Record
	require.NoError(t, db.Where("client = ?", "node").First(&saved).Error)
	require.Equal(t, int64(175), saved.NetTotalUp)
	require.Equal(t, int64(320), saved.NetTotalDown)
}

func TestSaveClientReportSkipsInvalidAgentTotals(t *testing.T) {
	resetReportCache(t)
	db := openReportCacheTestDB(t)
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	Records.Set("node", []reportmodel.Report{
		{UpdatedAt: now.Add(-30 * time.Second), Network: reportmodel.NetworkReport{TotalUp: -1, TotalDown: 200}},
		{UpdatedAt: now.Add(-5 * time.Second), Network: reportmodel.NetworkReport{TotalUp: 10, TotalDown: 20}},
	}, cache.DefaultExpiration)

	require.NoError(t, saveClientReportToDB(db, now))
	var saved models.Record
	require.NoError(t, db.Where("client = ?", "node").First(&saved).Error)
	require.Equal(t, int64(10), saved.NetTotalUp)
	require.Equal(t, int64(20), saved.NetTotalDown)
}
