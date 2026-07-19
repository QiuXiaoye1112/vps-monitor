package admin

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/monitor-monitor/monitor/cmd/flags"
	"github.com/monitor-monitor/monitor/database/dbcore"
	"github.com/monitor-monitor/monitor/database/models"
)

func TestMain(m *testing.M) {
	flags.DatabaseType = flags.DatabaseTypeSQLite
	flags.DatabaseFile = "file:web_api_admin_test?mode=memory&cache=shared"
	os.Exit(m.Run())
}

func TestUploadThemeBackground(t *testing.T) {
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("slot", "light"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("image", "background.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(generateTestPNG()); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/admin/theme/background", UploadThemeBackground)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/theme/background", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("upload returned %d: %s", w.Code, w.Body.String())
	}

	var themeCfg models.ThemeConfiguration
	if err := dbcore.GetDBInstance().Where("short = ?", "VPS").First(&themeCfg).Error; err != nil {
		t.Fatal(err)
	}
	settings := map[string]any{}
	if err := json.Unmarshal([]byte(themeCfg.Data), &settings); err != nil {
		t.Fatal(err)
	}
	if settings["backgroundType"] != "image" {
		t.Fatalf("background type was not normalized: %#v", settings)
	}
	url, _ := settings["lightBackgroundUrl"].(string)
	if !strings.HasPrefix(url, "/background/light?v=") {
		t.Fatalf("unexpected uploaded background URL %q", url)
	}
}
