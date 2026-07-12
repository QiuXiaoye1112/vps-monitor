package jsonrpc

import "testing"

func TestClientCreateUpdateFromParamsKeepsCreateFields(t *testing.T) {
	update := clientCreateUpdateFromParams("node-1", map[string]interface{}{
		"name":                      "BWG",
		"group":                     "美西",
		"weight":                    float64(3),
		"hidden":                    true,
		"traffic_limit":             float64(500 * 1024 * 1024 * 1024),
		"traffic_limit_type":        "sum",
		"traffic_reset_day":         float64(12),
		"traffic_reset_hour":        float64(6),
		"traffic_compensation":      float64(-2 * 1024 * 1024 * 1024),
		"traffic_compensation_base": float64(12 * 1024 * 1024 * 1024),
		"traffic_reset_enabled":     false,
		"token":                     "must-not-pass-through",
	})

	if update["uuid"] != "node-1" {
		t.Fatalf("uuid = %v, want node-1", update["uuid"])
	}
	for _, key := range []string{
		"name",
		"group",
		"weight",
		"hidden",
		"traffic_limit",
		"traffic_limit_type",
		"traffic_reset_day",
		"traffic_reset_hour",
		"traffic_compensation",
		"traffic_compensation_base",
		"traffic_reset_enabled",
	} {
		if _, ok := update[key]; !ok {
			t.Fatalf("expected update field %q to be kept", key)
		}
	}
	if _, ok := update["token"]; ok {
		t.Fatal("token must not be accepted during client creation update")
	}
	if got, ok := update["weight"].(int); !ok || got != 3 {
		t.Fatalf("weight = %#v, want int(3)", update["weight"])
	}
	if got, ok := update["traffic_limit"].(int64); !ok || got != 500*1024*1024*1024 {
		t.Fatalf("traffic_limit = %#v, want int64 500GiB", update["traffic_limit"])
	}
	if got, ok := update["traffic_compensation"].(int64); !ok || got != -2*1024*1024*1024 {
		t.Fatalf("traffic_compensation = %#v, want int64 -2GiB", update["traffic_compensation"])
	}
	if got, ok := update["traffic_compensation_base"].(int64); !ok || got != 12*1024*1024*1024 {
		t.Fatalf("traffic_compensation_base = %#v, want int64 12GiB", update["traffic_compensation_base"])
	}
}

func TestCompensationForVisibleBaseKeepsExpectedDisplayTotal(t *testing.T) {
	gib := float64(1024 * 1024 * 1024)
	visibleBase := int64(1.12 * gib)
	userCompensation := int64(317.42 * gib)
	rawAfterSave := int64(45.58 * gib)

	storedCompensation := compensationForVisibleBase(visibleBase, userCompensation, rawAfterSave)
	finalDisplay := rawAfterSave + storedCompensation
	wantDisplay := visibleBase + userCompensation

	if finalDisplay != wantDisplay {
		t.Fatalf("final display = %d, want %d", finalDisplay, wantDisplay)
	}
	if storedCompensation == userCompensation {
		t.Fatal("stored compensation should be adjusted when the raw backend base changes")
	}
}
