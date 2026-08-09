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
)

func TestSaveAndServeBackgroundImage(t *testing.T) {
	oldDir := backgroundStorageDir
	backgroundStorageDir = t.TempDir()
	t.Cleanup(func() { backgroundStorageDir = oldDir })

	pngData := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0x01}, 32)...)
	url, err := SaveBackgroundImage("light", pngData)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(url, "/background/light?v=") {
		t.Fatalf("unexpected background URL %q", url)
	}
	if _, err := os.Stat(filepath.Join(backgroundStorageDir, "light.png")); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/background/:slot", serveBackgroundImage)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s returned %d", url, w.Code)
	}
	if !bytes.Equal(w.Body.Bytes(), pngData) {
		t.Fatal("served background does not match uploaded image")
	}
	if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, "image/png") {
		t.Fatalf("unexpected Content-Type %q", got)
	}
	if got := w.Header().Get("Cache-Control"); got != immutableAssetCacheControl {
		t.Fatalf("unexpected Cache-Control %q", got)
	}
}

func TestBackgroundImageValidationAndVideoMigration(t *testing.T) {
	if _, err := SaveBackgroundImage("../dark", []byte("not an image")); err == nil {
		t.Fatal("unsafe slot was accepted")
	}
	if _, _, err := detectBackgroundImage([]byte("not an image")); err == nil {
		t.Fatal("non-image upload was accepted")
	}

	settings := map[string]any{
		"backgroundEnabled":  true,
		"backgroundType":     "video",
		"lightBackgroundUrl": "https://example.test/a.mp4",
		"darkBackgroundUrl":  "https://example.test/b.mp4",
	}
	NormalizeVPSBackgroundSettings(settings)
	if settings["backgroundType"] != "image" || settings["backgroundEnabled"] != false {
		t.Fatalf("video settings were not disabled: %#v", settings)
	}
	if settings["lightBackgroundUrl"] != "" || settings["darkBackgroundUrl"] != "" {
		t.Fatalf("retired video URLs were not cleared: %#v", settings)
	}

	remoteImage := map[string]any{
		"backgroundEnabled":  true,
		"backgroundType":     "image",
		"lightBackgroundUrl": "https://cdn.example.test/background.webp",
	}
	NormalizeVPSBackgroundSettings(remoteImage)
	if remoteImage["backgroundEnabled"] != false || remoteImage["lightBackgroundUrl"] != "" {
		t.Fatalf("remote background URL was not retired: %#v", remoteImage)
	}
}
