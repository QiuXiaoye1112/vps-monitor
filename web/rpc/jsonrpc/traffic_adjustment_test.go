package jsonrpc

import "testing"

func TestAdjustedTrafficTotalsNeverReturnsNegativeValues(t *testing.T) {
	tests := []struct {
		name         string
		up, down     int64
		compensation int64
		wantUp       int64
		wantDown     int64
	}{
		{name: "no compensation", up: 100, down: 20, wantUp: 100, wantDown: 20},
		{name: "positive compensation", up: 100, down: 20, compensation: 11, wantUp: 106, wantDown: 25},
		{name: "negative compensation transfers deficit", up: 100, down: 0, compensation: -50, wantUp: 50, wantDown: 0},
		{name: "compensation exceeds total", up: 10, down: 5, compensation: -20, wantUp: 0, wantDown: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotUp, gotDown := adjustedTrafficTotals(test.up, test.down, test.compensation)
			if gotUp != test.wantUp || gotDown != test.wantDown {
				t.Fatalf("got (%d, %d), want (%d, %d)", gotUp, gotDown, test.wantUp, test.wantDown)
			}
			if gotUp < 0 || gotDown < 0 {
				t.Fatalf("traffic totals must not be negative: (%d, %d)", gotUp, gotDown)
			}
		})
	}
}
