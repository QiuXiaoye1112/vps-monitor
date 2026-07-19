package public

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/monitor-monitor/monitor/cmd/flags"
	"github.com/monitor-monitor/monitor/database/dbcore"
	"github.com/monitor-monitor/monitor/database/models"
)

func TestMain(m *testing.M) {
	flags.DatabaseType = flags.DatabaseTypeSQLite
	flags.DatabaseFile = "file:web_public_test?mode=memory&cache=shared"

	db := dbcore.GetDBInstance()
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}

	os.Exit(m.Run())
}

func TestNormalizeHTMLLanguage(t *testing.T) {
	tests := map[string]struct {
		input string
		want  string
	}{
		"hyphen language": {
			input: "zh-CN",
			want:  "zh-CN",
		},
		"underscore language": {
			input: "zh_CN",
			want:  "zh-CN",
		},
		"reject script injection": {
			input: `zh-CN" autofocus`,
		},
		"reject too short": {
			input: "z",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := normalizeHTMLLanguage(tt.input); got != tt.want {
				t.Fatalf("normalizeHTMLLanguage(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestReplaceHTMLLanguage(t *testing.T) {
	tests := map[string]struct {
		html     string
		language string
		want     string
	}{
		"replace existing lang": {
			html:     `<html lang="en"><head></head></html>`,
			language: "zh-CN",
			want:     `<html lang="zh-CN"><head></head></html>`,
		},
		"insert missing lang": {
			html:     `<html><head></head></html>`,
			language: "ja_JP",
			want:     `<html lang="ja-JP"><head></head></html>`,
		},
		"ignore invalid lang": {
			html:     `<html lang="en"><head></head></html>`,
			language: `zh-CN" autofocus`,
			want:     `<html lang="en"><head></head></html>`,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := replaceHTMLLanguage(tt.html, tt.language); got != tt.want {
				t.Fatalf("replaceHTMLLanguage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInjectVPSThemeBootstrap(t *testing.T) {
	db := dbcore.GetDBInstance()
	t.Cleanup(func() {
		db.Where("short = ?", VpsTheme).Delete(&models.ThemeConfiguration{})
	})

	configuration := models.ThemeConfiguration{
		Short: VpsTheme,
		Data:  `{"backgroundEnabled":true,"backgroundType":"image","darkBackgroundUrl":"https://cdn.example/dark.webp?x=</script>"}`,
	}
	if err := db.Save(&configuration).Error; err != nil {
		t.Fatal(err)
	}

	got := injectVPSThemeBootstrap(`<head>`+vpsThemeBootstrapMarker+`</head>`, "Test Monitor")
	for _, required := range []string{
		`window.__VPS_THEME_BOOTSTRAP__=`,
		`"backgroundEnabled":true`,
		`"sitename":"Test Monitor"`,
		`\u003c/script\u003e`,
	} {
		if !strings.Contains(got, required) {
			t.Fatalf("bootstrap output missing %q: %s", required, got)
		}
	}
	if strings.Contains(got, vpsThemeBootstrapMarker) {
		t.Fatalf("bootstrap marker was not replaced: %s", got)
	}
}

func TestServeAvatar(t *testing.T) {
	avatarDir := "./data/avatar"
	_ = os.MkdirAll(avatarDir, 0755)
	avatarFile := filepath.Join(avatarDir, "ethan-avatar.png")
	testContent := []byte("fake png content")
	_ = os.WriteFile(avatarFile, testContent, 0644)
	defer os.Remove(avatarFile)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	Static(r.Group("/"), func(handlers ...gin.HandlerFunc) {
		r.NoRoute(handlers...)
	})

	req, _ := http.NewRequest("GET", "/images/ethan-avatar.png", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d, body: %s", w.Code, w.Body.String())
	}
	if !bytes.Equal(w.Body.Bytes(), testContent) {
		t.Fatalf("Expected body to match testContent")
	}
}

func TestUnknownAPIAndStaticAssetReturn404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	Static(r.Group("/"), func(handlers ...gin.HandlerFunc) {
		r.NoRoute(handlers...)
	})

	for _, requestPath := range []string{"/api/admin/task/all", "/assets/missing.js", "/styles/missing.css"} {
		req := httptest.NewRequest(http.MethodGet, requestPath, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("GET %s returned %d, want 404", requestPath, w.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/instance/test-node", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("SPA route returned %d, want 200", w.Code)
	}
}
