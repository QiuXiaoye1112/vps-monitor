package cmd

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/monitor-monitor/monitor/cmd/flags"
	"github.com/monitor-monitor/monitor/pkg/corn"
	"github.com/monitor-monitor/monitor/web/api"

	"github.com/monitor-monitor/monitor/database"
	"github.com/monitor-monitor/monitor/database/accounts"
	"github.com/monitor-monitor/monitor/database/auditlog"
	"github.com/monitor-monitor/monitor/database/clients"
	"github.com/monitor-monitor/monitor/database/dbcore"
	"github.com/monitor-monitor/monitor/database/models"
	"github.com/monitor-monitor/monitor/database/records"
	"github.com/monitor-monitor/monitor/database/tasks"
	"github.com/monitor-monitor/monitor/pkg/config"
	"github.com/monitor-monitor/monitor/utils"
	"github.com/monitor-monitor/monitor/utils/cloudflared"
	"github.com/monitor-monitor/monitor/utils/geoip"
	logutil "github.com/monitor-monitor/monitor/utils/log"
	"github.com/monitor-monitor/monitor/web/oauth"
	report_cache "github.com/monitor-monitor/monitor/web/report"
	"github.com/monitor-monitor/monitor/web/router"
	"github.com/monitor-monitor/monitor/web/security"
	"github.com/spf13/cobra"
)

var ServerCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the server",
	Long:  `Start the server`,
	Run: func(cmd *cobra.Command, args []string) {
		RunServer()
	},
}

func init() {
	// 从环境变量获取监听地址
	listenAddr := GetEnv("MONITOR_LISTEN", "0.0.0.0:25774")
	ServerCmd.PersistentFlags().StringVarP(&flags.Listen, "listen", "l", listenAddr, "监听地址 [env: MONITOR_LISTEN]")
	RootCmd.AddCommand(ServerCmd)
}

func RunServer() {
	// #region 初始化
	if err := os.MkdirAll("./data/theme", os.ModePerm); err != nil {
		log.Fatalf("Failed to create theme directory: %v", err)
	}
	InitDatabase()
	if utils.VersionHash != "unknown" {
		gin.SetMode(gin.ReleaseMode)
	}
	conf, err := config.GetManyAs[config.Settings]()
	if err != nil {
		log.Fatal(err)
	}
	go geoip.InitGeoIp()
	go DoScheduledWork()
	// oidcInit
	go oauth.Initialize()

	config.Subscribe(func(event config.ConfigEvent) {
		if ok, t := config.IsChangedT[string](event, config.OAuthProviderKey); ok {
			if t == "" || t == "none" {
				t = "github"
			}
			oidcProvider, err := database.GetOidcConfigByName(t)
			if err != nil {
				log.Printf("Failed to get OIDC provider config: %v", err)
			} else {
				log.Printf("Using %s as OIDC provider", oidcProvider.Name)
			}
			err = oauth.LoadProvider(oidcProvider.Name, oidcProvider.Addition)
			if err != nil {
				auditlog.EventLog("error", fmt.Sprintf("Failed to load OIDC provider: %v", err))
			}
		}

	})
	// 初始化 cloudflared
	if err := cloudflared.AutoStart(GetEnv("MONITOR_CLOUDFLARED_TOKEN", "")); err != nil {
		log.Printf("failed to auto start cloudflared: %v", err)
	}

	r := gin.New()
	r.Use(logutil.GinLogger())
	r.Use(logutil.GinRecovery())

	config.Subscribe(func(event config.ConfigEvent) {
		if event.IsChanged(config.GeoIpProviderKey) {
			go geoip.InitGeoIp()
		}

	})
	r.Use(security.CorsMiddleware(conf.CorsOriginCheckEnabled, conf.CorsAllowedOrigins))

	r.Use(api.IdentityMiddleware())

	r.Use(func(c *gin.Context) {
		// Dynamic responses and unhashed files must never be cached. The static
		// theme handler explicitly overrides this only for hashed /assets JS/CSS.
		c.Header("Cache-Control", "no-store")
		c.Next()
	})

	router.Register(r)

	srv := &http.Server{
		Addr:    flags.Listen,
		Handler: r,
	}
	log.Printf("Starting server on %s ...", flags.Listen)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			OnFatal(err)
			log.Fatalf("listen: %s\n", err)
		}
	}()
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	OnShutdown()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

}

func InitDatabase() {
	var count int64 = 0
	if dbcore.GetDBInstance().Model(&models.User{}).Count(&count); count == 0 {
		user, passwd, err := accounts.CreateDefaultAdminAccount()
		if err != nil {
			panic(err)
		}
		log.Println("Default admin account created. Username:", user, ", Password:", passwd)
	}
}

// #region 定时任务
func DoScheduledWork() {
	if err := tasks.ReloadPingSchedule(); err != nil {
		log.Println("Failed to reload ping schedule:", err)
	}
	records.CompactRecord()

	if err := corn.AddFunc("records:cleanup", "@every 30m", cleanupScheduledData); err != nil {
		log.Println("Failed to add cleanup scheduled task:", err)
	}
	if err := corn.AddFunc("records:minute", "@every 1m", minuteScheduledWork); err != nil {
		log.Println("Failed to add minute scheduled task:", err)
	}
	if err := corn.AddFunc("records:history", "@every 2s", historyScheduledWork); err != nil {
		log.Println("Failed to add history persistence task:", err)
	}
}

func cleanupScheduledData() {
	clients.ResetTrafficCompensationForDueClients()
	records.CompactRecord()
	records.DeleteRecordBefore(time.Now().Add(-time.Hour * time.Duration(config.DefaultRecordPreserveTime)))
	records.DeleteHistoryBefore(time.Now().Add(-7 * 24 * time.Hour))
	tasks.ClearTaskResultsByTimeBefore(time.Now().Add(-time.Hour * time.Duration(config.DefaultRecordPreserveTime)))
	tasks.DeletePingRecordsBefore(time.Now().Add(-time.Hour * time.Duration(config.DefaultPingRecordPreserveTime)))
	auditlog.RemoveOldLogs()
	accounts.RemoveExpiredSessions()
}

func minuteScheduledWork() {
	cfg, _ := config.GetManyAs[config.Settings]()
	report_cache.SaveClientReportToDB()
	if !cfg.RecordEnabled {
		records.DeleteAll()
		tasks.DeleteAllPingRecords()
	}
	clients.ResetTrafficCompensationForDueClients()
}

func historyScheduledWork() {
	if err := report_cache.SaveHistoryReportsToDB(); err != nil {
		log.Printf("Failed to persist history reports: %v", err)
	}
}

func OnShutdown() {
	auditlog.Log("", "", "server is shutting down", "info")
	corn.StopAll()
	if err := report_cache.SaveHistoryReportsToDB(); err != nil {
		log.Printf("Failed to flush history reports on shutdown: %v", err)
	}
	cloudflared.Shutdown()
}

func OnFatal(err error) {
	auditlog.Log("", "", "server encountered a fatal error: "+err.Error(), "error")
	cloudflared.Shutdown()
}
