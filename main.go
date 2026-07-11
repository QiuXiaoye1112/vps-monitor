package main

import (
	"log"
	"log/slog"

	"github.com/monitor-monitor/monitor/cmd"
	"github.com/monitor-monitor/monitor/utils"
	logutil "github.com/monitor-monitor/monitor/utils/log"
)

func main() {
	if utils.VersionHash == "unknown" {
		logutil.SetupGlobalLogger(slog.LevelDebug)
	} else {
		logutil.SetupGlobalLogger(slog.LevelInfo)
	}

	log.Printf("VPS Monitor %s (hash: %s)", utils.CurrentVersion, utils.VersionHash)

	cmd.Execute()
}
