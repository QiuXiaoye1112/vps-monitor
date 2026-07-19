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
		`data-vps-custom-background="true"] .default-background`,
		`data-vps-custom-background="true"] #vps-background-bootstrap`,
		`data-vps-custom-background="true"] .loading-cover`,
		`background-color: transparent !important`,
		`id="vps-background-bootstrap"`,
		`new MutationObserver(detectAppBackground)`,
		`document.querySelector('.loading-cover')`,
		`image.style.backgroundImage`,
		`requestAnimationFrame(() => requestAnimationFrame(releaseBootstrap))`,
		`background-image-dark-fix-20260719`,
		`href="/assets/index-zYil-n0Q.css?v=background-image-dark-fix-20260719"`,
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
	raw, err := PublicFS.ReadFile("vpsTheme/dist/assets/index-BmdGcRWR-vpsbundle1449.js")
	if err != nil {
		t.Fatal(err)
	}
	bundle := string(raw)
	for _, required := range []string{
		`I=k(()=>"image")`,
		`F=k(()=>"dark"===D.value?A.value||O.value:O.value)`,
		`t.backgroundEnabled&&!!t.currentBackgroundUrl`,
	} {
		if !strings.Contains(bundle, required) {
			t.Fatalf("image-only background runtime guard %q is missing", required)
		}
	}
}

func TestVPSThemeDefaultDarkBackgroundIsOpaque(t *testing.T) {
	raw, err := PublicFS.ReadFile("vpsTheme/dist/assets/index-zYil-n0Q.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(raw)
	if !strings.Contains(css, `.dark .default-background[data-v-f4798363]{background:#0f172a}`) {
		t.Fatal("default dark background must use an opaque base color")
	}
	if strings.Contains(css, `.dark .default-background[data-v-f4798363]{background:#0f172a80}`) {
		t.Fatal("translucent default dark background must not be restored")
	}
}
