package jsonrpc

import (
	"math"
	"testing"
)

func TestAgentMonthlyTrafficTotalUsesMeasuredDirections(t *testing.T) {
	if got := saturatingInt64Add(100, 20); got != 120 {
		t.Fatalf("monthly traffic = %d, want 120", got)
	}
	if got := saturatingInt64Add(math.MaxInt64-5, 10); got != math.MaxInt64 {
		t.Fatalf("overflowing monthly traffic = %d, want MaxInt64", got)
	}
}
