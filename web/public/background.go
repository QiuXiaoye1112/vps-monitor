package public

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const MaxBackgroundImageBytes = 10 << 20 // 10 MiB

var backgroundStorageDir = filepath.Join(DataDir, "background")

var backgroundImageFormats = []struct {
	ext      string
	mimeType string
	match    func([]byte) bool
}{
	{ext: ".png", mimeType: "image/png", match: func(data []byte) bool {
		return len(data) >= 8 && bytes.Equal(data[:8], []byte("\x89PNG\r\n\x1a\n"))
	}},
	{ext: ".jpg", mimeType: "image/jpeg", match: func(data []byte) bool {
		return len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff
	}},
	{ext: ".gif", mimeType: "image/gif", match: func(data []byte) bool {
		return len(data) >= 6 && (bytes.Equal(data[:6], []byte("GIF87a")) || bytes.Equal(data[:6], []byte("GIF89a")))
	}},
	{ext: ".webp", mimeType: "image/webp", match: func(data []byte) bool {
		return len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP"))
	}},
	{ext: ".avif", mimeType: "image/avif", match: func(data []byte) bool {
		if len(data) < 16 || !bytes.Equal(data[4:8], []byte("ftyp")) {
			return false
		}
		header := data
		if len(header) > 64 {
			header = header[:64]
		}
		return bytes.Contains(header[8:], []byte("avif")) || bytes.Contains(header[8:], []byte("avis"))
	}},
}

func validBackgroundSlot(slot string) bool {
	return slot == "light" || slot == "dark"
}

func detectBackgroundImage(data []byte) (string, string, error) {
	if len(data) == 0 {
		return "", "", errors.New("图片文件为空")
	}
	if len(data) > MaxBackgroundImageBytes {
		return "", "", fmt.Errorf("图片不能超过 %d MB", MaxBackgroundImageBytes>>20)
	}
	for _, format := range backgroundImageFormats {
		if format.match(data) {
			return format.ext, format.mimeType, nil
		}
	}
	return "", "", errors.New("仅支持 PNG、JPG、GIF、WebP 或 AVIF 图片")
}

func backgroundImageDir() string {
	return backgroundStorageDir
}

func backgroundImageFile(slot string) (string, string, bool) {
	if !validBackgroundSlot(slot) {
		return "", "", false
	}
	for _, format := range backgroundImageFormats {
		filePath := filepath.Join(backgroundImageDir(), slot+format.ext)
		if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
			return filePath, format.mimeType, true
		}
	}
	return "", "", false
}

// SaveBackgroundImage validates and atomically replaces one locally managed
// theme background. The returned URL is versioned so browsers never reuse the
// previous image after an upload.
func SaveBackgroundImage(slot string, data []byte) (string, error) {
	if !validBackgroundSlot(slot) {
		return "", errors.New("背景位置无效")
	}
	ext, _, err := detectBackgroundImage(data)
	if err != nil {
		return "", err
	}

	dir := backgroundImageDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建背景目录失败: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "."+slot+"-*.tmp")
	if err != nil {
		return "", fmt.Errorf("创建背景临时文件失败: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("写入背景图片失败: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("同步背景图片失败: %w", err)
	}
	if err := tmp.Chmod(0644); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("设置背景图片权限失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("关闭背景图片失败: %w", err)
	}

	finalPath := filepath.Join(dir, slot+ext)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return "", fmt.Errorf("更新背景图片失败: %w", err)
	}
	for _, format := range backgroundImageFormats {
		oldPath := filepath.Join(dir, slot+format.ext)
		if oldPath != finalPath {
			_ = os.Remove(oldPath)
		}
	}
	info, err := os.Stat(finalPath)
	if err != nil {
		return "", fmt.Errorf("读取背景图片状态失败: %w", err)
	}
	return fmt.Sprintf("/background/%s?v=%d", slot, info.ModTime().UnixNano()), nil
}

func serveBackgroundImage(c *gin.Context) {
	slot := strings.TrimSpace(c.Param("slot"))
	filePath, mimeType, ok := backgroundImageFile(slot)
	if !ok {
		c.Status(404)
		return
	}
	info, err := os.Stat(filePath)
	if err != nil {
		c.Status(404)
		return
	}
	c.Header("Cache-Control", noStoreCacheControl)
	c.Header("Content-Type", mimeType)
	c.Header("Last-Modified", info.ModTime().UTC().Format(time.RFC1123))
	c.File(filePath)
}

// NormalizeVPSBackgroundSettings removes the retired video/remote URL modes
// from every public/settings response. Only files created by the local upload
// endpoint are accepted as theme backgrounds.
func NormalizeVPSBackgroundSettings(settings map[string]any) {
	if settings == nil {
		return
	}
	lightURL, _ := settings["lightBackgroundUrl"].(string)
	darkURL, _ := settings["darkBackgroundUrl"].(string)
	if !strings.HasPrefix(lightURL, "/background/light?v=") {
		lightURL = ""
	}
	if !strings.HasPrefix(darkURL, "/background/dark?v=") {
		darkURL = ""
	}
	settings["lightBackgroundUrl"] = lightURL
	settings["darkBackgroundUrl"] = darkURL
	if lightURL == "" && darkURL == "" {
		settings["backgroundEnabled"] = false
	}
	settings["backgroundType"] = "image"
}
