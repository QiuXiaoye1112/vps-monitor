package clients

import (
	"testing"
	"time"

	"github.com/monitor-monitor/monitor/database/models"
)

func TestShouldResetTrafficCompensation(t *testing.T) {
	loc := time.FixedZone("Asia/Shanghai", 8*60*60)
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, loc)
	old := models.FromTime(time.Date(2026, 6, 15, 12, 0, 0, 0, loc))
	current := models.FromTime(time.Date(2026, 7, 5, 12, 0, 0, 0, loc))

	base := models.Client{
		TrafficComp:         1024,
		TrafficResetDay:     1,
		TrafficResetHour:    0,
		TrafficResetEnabled: true,
		TrafficCompResetAt:  old,
	}

	if !shouldResetTrafficCompensation(base, now) {
		t.Fatal("expected old compensation to reset when monthly reset is enabled")
	}

	disabled := base
	disabled.TrafficResetEnabled = false
	if shouldResetTrafficCompensation(disabled, now) {
		t.Fatal("compensation must not reset when monthly reset is disabled")
	}

	updatedThisCycle := base
	updatedThisCycle.TrafficCompResetAt = current
	if shouldResetTrafficCompensation(updatedThisCycle, now) {
		t.Fatal("compensation updated in the current cycle must not reset")
	}

	zero := base
	zero.TrafficComp = 0
	if shouldResetTrafficCompensation(zero, now) {
		t.Fatal("zero compensation does not need a reset")
	}

	carryOnly := zero
	carryOnly.TrafficCarry = 2048
	if !shouldResetTrafficCompensation(carryOnly, now) {
		t.Fatal("internal carry must reset at the same monthly boundary")
	}
}
