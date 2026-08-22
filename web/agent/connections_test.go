package agent

import (
	"testing"
	"time"

	report "github.com/monitor-monitor/monitor/protocol/report"
)

func TestSetLatestReportRejectsOlderTrafficSequence(t *testing.T) {
	const uuid = "ordered-report-client"
	DeleteLatestReport(uuid)
	t.Cleanup(func() { DeleteLatestReport(uuid) })

	base := time.Date(2026, time.August, 23, 0, 0, 0, 0, time.UTC)
	if !SetLatestReport(uuid, &report.Report{Network: report.NetworkReport{
		CycleGeneration: 4,
		SampleSequence:  10,
		CapturedAt:      base,
		TotalUp:         100,
	}}) {
		t.Fatal("initial report was rejected")
	}
	if SetLatestReport(uuid, &report.Report{Network: report.NetworkReport{
		CycleGeneration: 4,
		SampleSequence:  9,
		CapturedAt:      base.Add(time.Second),
		TotalUp:         90,
	}}) {
		t.Fatal("older report was accepted")
	}
	latest := GetLatestReport()[uuid]
	if latest == nil || latest.Network.TotalUp != 100 {
		t.Fatalf("latest report was overwritten: %+v", latest)
	}
	if !SetLatestReport(uuid, &report.Report{Network: report.NetworkReport{
		CycleGeneration: 4,
		SampleSequence:  11,
		CapturedAt:      base.Add(2 * time.Second),
		TotalUp:         110,
	}}) {
		t.Fatal("newer report was rejected")
	}
}

func TestSetLatestReportRejectsReportFromBeforeReset(t *testing.T) {
	const uuid = "reset-generation-client"
	DeleteLatestReport(uuid)
	t.Cleanup(func() { DeleteLatestReport(uuid) })

	if !SetLatestReport(uuid, &report.Report{Network: report.NetworkReport{
		CycleGeneration: 8,
		SampleSequence:  20,
		TotalUp:         500,
	}}) {
		t.Fatal("pre-reset report was rejected")
	}
	if !SetLatestReport(uuid, &report.Report{Network: report.NetworkReport{
		CycleGeneration: 9,
		SampleSequence:  1,
		TotalUp:         0,
	}}) {
		t.Fatal("post-reset report was rejected")
	}
	if SetLatestReport(uuid, &report.Report{Network: report.NetworkReport{
		CycleGeneration: 8,
		SampleSequence:  21,
		TotalUp:         520,
	}}) {
		t.Fatal("pre-reset report was accepted after reset")
	}
	latest := GetLatestReport()[uuid]
	if latest == nil || latest.Network.CycleGeneration != 9 || latest.Network.TotalUp != 0 {
		t.Fatalf("post-reset report was overwritten: %+v", latest)
	}
}

func TestSetLatestReportSwitchesToRebuiltLedgerEpoch(t *testing.T) {
	const uuid = "rebuilt-ledger-client"
	DeleteLatestReport(uuid)
	t.Cleanup(func() { DeleteLatestReport(uuid) })

	oldCapture := time.Date(2026, time.August, 23, 0, 0, 0, 0, time.UTC)
	if !SetLatestReport(uuid, &report.Report{Network: report.NetworkReport{
		LedgerEpoch:     "1700000000000000000-old-epoch",
		CycleGeneration: 12,
		SampleSequence:  99,
		CapturedAt:      oldCapture,
		TotalUp:         900,
	}}) {
		t.Fatal("initial epoch report was rejected")
	}
	if !SetLatestReport(uuid, &report.Report{Network: report.NetworkReport{
		LedgerEpoch:     "1700000000000001000-new-epoch",
		CycleGeneration: 1,
		SampleSequence:  1,
		CapturedAt:      oldCapture.Add(time.Minute),
		TotalUp:         0,
	}}) {
		t.Fatal("rebuilt ledger epoch was rejected")
	}
	if SetLatestReport(uuid, &report.Report{Network: report.NetworkReport{
		LedgerEpoch:     "1700000000000000000-old-epoch",
		CycleGeneration: 12,
		SampleSequence:  100,
		CapturedAt:      oldCapture.Add(2 * time.Minute),
		TotalUp:         920,
	}}) {
		t.Fatal("old ledger epoch was accepted after rebuild")
	}
}
