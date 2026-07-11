package public

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestVPSThemeManifestDoesNotExposeRemovedSettings(t *testing.T) {
	raw, err := PublicFS.ReadFile("vpsTheme/monitor-theme.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Configuration struct {
			Data []struct {
				Key string `json:"key"`
			} `json:"data"`
		} `json:"configuration"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	removed := map[string]bool{"rpcTransportMode": true, "disablePageAnimation": true}
	for _, item := range manifest.Configuration.Data {
		if removed[item.Key] {
			t.Fatalf("removed theme setting %q is still exposed", item.Key)
		}
	}
}

func TestVPSAdminDoesNotExposeTaskHistoryAndUsesSafeDefaults(t *testing.T) {
	raw, err := PublicFS.ReadFile("vpsTheme/dist/admin.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(raw)
	for _, removed := range []string{"任务记录", "admin:getTasks", "tasksPage", "rpcTransportMode", "disablePageAnimation"} {
		if strings.Contains(html, removed) {
			t.Fatalf("removed admin feature %q is still present", removed)
		}
	}
	for _, required := range []string{
		`stopEarth:false`,
		`earthRenderer:"cobe"`,
		`hideEarth:false`,
		`backgroundEnabled:false`,
		`Math.max(0,(Date.now()-ms)/1000)`,
		`return"刚刚"`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("expected admin regression guard %q", required)
		}
	}
}
