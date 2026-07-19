package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/monitor-monitor/monitor/database/dbcore"
	"github.com/monitor-monitor/monitor/database/models"
	"github.com/monitor-monitor/monitor/web/api"
	"github.com/monitor-monitor/monitor/web/public"
)

func UploadThemeBackground(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, public.MaxBackgroundImageBytes+(1<<20))
	if err := c.Request.ParseMultipartForm(public.MaxBackgroundImageBytes); err != nil {
		status := http.StatusBadRequest
		message := "读取上传图片失败"
		if strings.Contains(strings.ToLower(err.Error()), "too large") {
			status = http.StatusRequestEntityTooLarge
			message = "图片不能超过 10 MB"
		}
		api.RespondError(c, status, message)
		return
	}

	slot := strings.TrimSpace(c.PostForm("slot"))
	if slot != "light" && slot != "dark" {
		api.RespondError(c, http.StatusBadRequest, "背景位置无效")
		return
	}
	fileHeader, err := c.FormFile("image")
	if err != nil {
		api.RespondError(c, http.StatusBadRequest, "请选择要上传的图片")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		api.RespondError(c, http.StatusBadRequest, "无法打开上传图片")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, public.MaxBackgroundImageBytes+1))
	if err != nil {
		api.RespondError(c, http.StatusBadRequest, "读取上传图片失败")
		return
	}
	if len(data) > public.MaxBackgroundImageBytes {
		api.RespondError(c, http.StatusRequestEntityTooLarge, "图片不能超过 10 MB")
		return
	}

	imageURL, err := public.SaveBackgroundImage(slot, data)
	if err != nil {
		api.RespondError(c, http.StatusBadRequest, err.Error())
		return
	}

	db := dbcore.GetDBInstance()
	var themeCfg models.ThemeConfiguration
	if err := db.Where("short = ?", public.VpsTheme).First(&themeCfg).Error; err != nil {
		themeCfg = models.ThemeConfiguration{Short: public.VpsTheme}
	}
	settings := map[string]any{}
	if themeCfg.Data != "" {
		_ = json.Unmarshal([]byte(themeCfg.Data), &settings)
	}
	public.NormalizeVPSBackgroundSettings(settings)
	if slot == "light" {
		settings["lightBackgroundUrl"] = imageURL
	} else {
		settings["darkBackgroundUrl"] = imageURL
	}
	settings["backgroundEnabled"] = true
	merged, err := json.Marshal(settings)
	if err != nil {
		api.RespondError(c, http.StatusInternalServerError, "保存背景配置失败")
		return
	}
	themeCfg.Data = string(merged)
	if err := db.Save(&themeCfg).Error; err != nil {
		api.RespondError(c, http.StatusInternalServerError, "保存背景配置失败: "+err.Error())
		return
	}

	api.RespondSuccessMessage(c, "背景图片已上传", gin.H{"path": imageURL, "slot": slot})
}
