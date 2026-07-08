package admin

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/web/api"
)

const avatarPath = "./data/avatar/ethan-avatar.png"

func UploadAvatar(c *gin.Context) {
	var req struct {
		Image string `json:"image" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		api.RespondError(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}

	payload := req.Image
	if comma := strings.IndexByte(payload, ','); comma >= 0 {
		payload = payload[comma+1:]
	}

	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil || len(raw) == 0 {
		api.RespondError(c, http.StatusBadRequest, "头像数据无效")
		return
	}
	if len(raw) > 2*1024*1024 {
		api.RespondError(c, http.StatusBadRequest, "头像不能超过 2MB")
		return
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		api.RespondError(c, http.StatusBadRequest, "不支持的图片格式")
		return
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 4096 || cfg.Height > 4096 {
		api.RespondError(c, http.StatusBadRequest, "图片尺寸无效")
		return
	}

	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		api.RespondError(c, http.StatusBadRequest, "图片解码失败")
		return
	}

	if err := os.MkdirAll(filepath.Dir(avatarPath), 0755); err != nil {
		api.RespondError(c, http.StatusInternalServerError, "创建头像目录失败")
		return
	}

	tmp := avatarPath + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		api.RespondError(c, http.StatusInternalServerError, "保存头像失败")
		return
	}

	encodeErr := png.Encode(f, img)
	closeErr := f.Close()
	if encodeErr != nil || closeErr != nil {
		_ = os.Remove(tmp)
		api.RespondError(c, http.StatusInternalServerError, "写入头像失败")
		return
	}

	if err := os.Rename(tmp, avatarPath); err != nil {
		_ = os.Remove(tmp)
		api.RespondError(c, http.StatusInternalServerError, "更新头像失败")
		return
	}

	api.RespondSuccessMessage(c, "头像已更新", gin.H{"path": "/images/ethan-avatar.png"})
}
