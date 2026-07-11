package database

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/monitor-monitor/monitor/database/dbcore"
	"github.com/monitor-monitor/monitor/database/models"
	"github.com/monitor-monitor/monitor/pkg/config"
	"github.com/monitor-monitor/monitor/web/public"
)

func GetPublicInfo() (map[string]interface{}, error) {
	cstPtr, err := config.GetManyAs[config.Settings]()
	if err != nil {
		return nil, err
	}
	cst := *cstPtr

	all, allErr := config.GetAll()
	hasKey := func(k string) bool {
		if allErr != nil {
			return false
		}
		_, ok := all[k]
		return ok
	}

	// Apply defaults only when a key is missing.
	if !hasKey("sitename") {
		cst.Sitename = "VPS Monitor"
	}
	if !hasKey("description") {
		cst.Description = "VPS Monitor"
	}
	if !hasKey("theme") {
		cst.Theme = "VPS"
	}
	if !hasKey("o_auth_provider") {
		cst.OAuthProvider = "github"
	}
	if !hasKey("record_enabled") {
		cst.RecordEnabled = true
	}
	if !hasKey("record_preserve_time") {
		cst.RecordPreserveTime = 720
	}
	if !hasKey("ping_record_preserve_time") {
		cst.PingRecordPreserveTime = config.DefaultPingRecordPreserveTime
	}
	if cst.PingRecordPreserveTime < config.DefaultPingRecordPreserveTime {
		cst.PingRecordPreserveTime = config.DefaultPingRecordPreserveTime
	}

	// Fallback defaults if we couldn't enumerate keys.
	if allErr != nil {
		if cst.Sitename == "" {
			cst.Sitename = "VPS Monitor"
		}
		if cst.Description == "" {
			cst.Description = "VPS Monitor"
		}
	}

	db := dbcore.GetDBInstance()
	tc := models.ThemeConfiguration{}
	err = db.Model(&models.ThemeConfiguration{}).Where("short = ?", cst.Theme).First(&tc).Error
	if err != nil {
		tc.Data = "{}"
	}
	tc_data := gin.H{}
	err = json.Unmarshal([]byte(tc.Data), &tc_data)
	if err != nil {
		log.Printf("%v", err)
	}
	// Try to load theme declaration file and merge defaults for managed configuration
	// Theme declarations live in ./data/theme/<short>/monitor-theme.json
	if cst.Theme != "" && cst.Theme != "default" {
		var b []byte
		var readErr error
		if cst.Theme == public.VpsTheme {
			b, readErr = public.PublicFS.ReadFile("vpsTheme/monitor-theme.json")
		} else {
			themeConfigPath := filepath.Join("./data/theme", cst.Theme, "monitor-theme.json")
			if _, err := os.Stat(themeConfigPath); err == nil {
				b, readErr = os.ReadFile(themeConfigPath)
			}
		}
		if readErr == nil && len(b) > 0 {
			var themeDecl struct {
				Configuration struct {
					Type string                                 `json:"type"`
					Data []models.ManagedThemeConfigurationItem `json:"data"`
				} `json:"configuration"`
			}
			if err := json.Unmarshal(b, &themeDecl); err == nil {
				if themeDecl.Configuration.Type == "managed" {
					for _, item := range themeDecl.Configuration.Data {
						if item.Key == "" {
							continue
						}
						// missing
						if _, exists := tc_data[item.Key]; !exists {
							var def any = item.Default
							// select
							if item.Type == "select" {
								if def == nil || def == "" {
									if item.Options != "" {
										opts := strings.Split(item.Options, ",")
										if len(opts) > 0 {
											def = strings.TrimSpace(opts[0])
										}
									}
								}
							}
							// number->0, string->"", switch->false
							if def == nil {
								switch item.Type {
								case "number":
									def = 0
								case "switch":
									def = false
								default:
									def = ""
								}
							}
							tc_data[item.Key] = def
						}
					}
				}
			}
		}
	}

	return gin.H{
		"sitename":                  cst.Sitename,
		"record_enabled":            cst.RecordEnabled,
		"record_preserve_time":      cst.RecordPreserveTime,
		"ping_record_preserve_time": cst.PingRecordPreserveTime,
		"theme":                     cst.Theme,
		"theme_settings":            tc_data,
	}, nil
}
