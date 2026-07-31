package public

import (
	"encoding/json"
	"io/fs"
	"strings"
	"testing"
)

func readVPSThemeAssets(t *testing.T, pattern string) string {
	t.Helper()
	paths, err := fs.Glob(PublicFS, pattern)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatalf("no theme assets matched %q", pattern)
	}
	var result strings.Builder
	for _, path := range paths {
		raw, err := PublicFS.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		result.Write(raw)
		result.WriteByte('\n')
	}
	return result.String()
}

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
	removed := map[string]bool{
		"rpcTransportMode":     true,
		"disablePageAnimation": true,
		"backgroundType":       true,
		"lightBackgroundUrl":   true,
		"darkBackgroundUrl":    true,
	}
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
	for _, removed := range []string{
		"任务记录",
		"admin:getTasks",
		"tasksPage",
		"rpcTransportMode",
		"disablePageAnimation",
		`l: "背景类型"`,
		`l: "亮色背景 URL"`,
		`l: "暗色背景 URL"`,
		`o: ["image", "video"]`,
	} {
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
		`id: "backgroundLightInput", slot: "light"`,
		`id: "backgroundDarkInput", slot: "dark"`,
		`/api/admin/theme/background`,
		`选择图片并上传`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("expected admin regression guard %q", required)
		}
	}
}

func TestVPSAdminCreateCannotOverrideServerAssignedWeight(t *testing.T) {
	raw, err := PublicFS.ReadFile("vpsTheme/dist/admin.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(raw)
	start := strings.Index(html, `var abtn = _d.getElementById("addNodeBtn")`)
	if start < 0 {
		t.Fatal("add-node handler is missing from embedded admin")
	}
	endOffset := strings.Index(html[start:], `var pbtn = _d.getElementById("addPingBtn")`)
	if endOffset < 0 {
		t.Fatal("could not determine embedded add-node handler boundary")
	}
	addNodeHandler := html[start : start+endOffset]
	for _, forbidden := range []string{`name="weight"`, `weight:`} {
		if strings.Contains(addNodeHandler, forbidden) {
			t.Fatalf("embedded add-node handler must not submit %q", forbidden)
		}
	}
}

func TestVPSThemeBootstrapsCustomBackgroundBeforeAppMount(t *testing.T) {
	raw, err := PublicFS.ReadFile("vpsTheme/dist/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(raw)
	for _, required := range []string{
		vpsThemeBootstrapMarker,
		`window.__VPS_BACKGROUND_READY__`,
		`rel = 'preload'`,
		`preload.as = 'image'`,
		`fetchPriority = 'high'`,
		`data-vps-custom-background='true'] .default-background`,
		`data-vps-custom-background='true'] #vps-background-bootstrap`,
		`data-vps-custom-background='true'] .loading-cover`,
		`background-color: transparent !important`,
		`backdrop-filter: blur(8px) !important`,
		`id="vps-background-bootstrap"`,
		`new MutationObserver(detectAppBackground)`,
		`document.querySelector('.loading-cover')`,
		`image.style.backgroundImage`,
		`requestAnimationFrame(() => requestAnimationFrame(releaseBootstrap))`,
		`type="module"`,
		`rel="stylesheet"`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("background bootstrap guard %q is missing", required)
		}
	}
	for _, removed := range []string{
		`settings.backgroundType === 'video'`,
		`document.createElement('video')`,
		`__VPS_BACKGROUND_BOOTSTRAP_VIDEO__`,
		`bootstrapVideo.pause()`,
	} {
		if strings.Contains(html, removed) {
			t.Fatalf("removed video bootstrap %q is still present", removed)
		}
	}
}

func TestVPSThemeRuntimeOnlyUsesImagesAndFallsBackToLightBackground(t *testing.T) {
	bundle := readVPSThemeAssets(t, "vpsTheme/dist/assets/*.js")
	for _, required := range []string{
		`background-image`,
		`__VPS_BACKGROUND_PRELOADED__`,
		`VPS Monitor`,
		`累计流量`,
		`流量重置时间`,
	} {
		if !strings.Contains(bundle, required) {
			t.Fatalf("image-only background runtime guard %q is missing", required)
		}
	}
	for _, removed := range []string{
		`background-video`,
		`Komari Glassmorphism`,
		`搜索节点`,
		`显示高级工具`,
	} {
		if strings.Contains(bundle, removed) {
			t.Fatalf("removed theme runtime %q is still present", removed)
		}
	}
}

func TestVPSThemeDefaultDarkBackgroundIsOpaque(t *testing.T) {
	css := readVPSThemeAssets(t, "vpsTheme/dist/assets/*.css")
	if !strings.Contains(css, `.dark .default-background`) || !strings.Contains(css, `background:#0f172a`) {
		t.Fatal("default dark background must use an opaque base color")
	}
	if strings.Contains(css, `background:#0f172a80`) {
		t.Fatal("translucent default dark background must not be restored")
	}
}
