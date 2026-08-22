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
		"traffic_reset_minute":      float64(37),
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
		"hidden",
		"traffic_limit",
		"traffic_limit_type",
		"traffic_reset_day",
		"traffic_reset_hour",
		"traffic_reset_minute",
		"traffic_reset_enabled",
	} {
		if _, ok := update[key]; !ok {
			t.Fatalf("expected update field %q to be kept", key)
		}
	}
	if _, ok := update["traffic_compensation"]; ok {
		t.Fatal("traffic_compensation must not be accepted")
	}
	if _, ok := update["traffic_compensation_base"]; ok {
		t.Fatal("traffic_compensation_base must not be accepted")
	}
	if _, ok := update["token"]; ok {
		t.Fatal("token must not be accepted during client creation update")
	}
	if _, ok := update["weight"]; ok {
		t.Fatal("weight must remain server-controlled during client creation")
	}
	if got, ok := update["traffic_limit"].(int64); !ok || got != 500*1024*1024*1024 {
		t.Fatalf("traffic_limit = %#v, want int64 500GiB", update["traffic_limit"])
	}
	if got, ok := update["traffic_reset_minute"].(int); !ok || got != 37 {
		t.Fatalf("traffic_reset_minute = %#v, want int(37)", update["traffic_reset_minute"])
	}
}
