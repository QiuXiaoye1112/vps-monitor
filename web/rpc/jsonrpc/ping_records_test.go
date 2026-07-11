package jsonrpc

import (
	"testing"
	"time"
)

func TestClampPingRecordRangeLimitsWindowToSevenDays(t *testing.T) {
	end := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	start := end.Add(-10 * 24 * time.Hour)

	gotStart, gotEnd := clampPingRecordRange(start, end)

	if !gotEnd.Equal(end) {
		t.Fatalf("end time changed: got %s, want %s", gotEnd, end)
	}
	if got := gotEnd.Sub(gotStart); got != maxPingRecordWindow {
		t.Fatalf("window = %s, want %s", got, maxPingRecordWindow)
	}
}

func TestClampPingRecordRangeKeepsShortWindow(t *testing.T) {
	end := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	start := end.Add(-3 * time.Hour)

	gotStart, gotEnd := clampPingRecordRange(start, end)

	if !gotStart.Equal(start) || !gotEnd.Equal(end) {
		t.Fatalf("range changed: got %s..%s, want %s..%s", gotStart, gotEnd, start, end)
	}
}
