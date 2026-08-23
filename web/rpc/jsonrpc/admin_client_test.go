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
		"traffic_compensation":      float64(-2 * 1024 * 1024 * 1024),
		"traffic_compensation_base": float64(12 * 1024 * 1024 * 1024),
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
}
