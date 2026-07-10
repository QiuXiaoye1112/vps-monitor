package cmd

import (
	"fmt"
	"os"

	"github.com/monitor-monitor/monitor/cmd/flags"

	"github.com/spf13/cobra"
)

func GetEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// 从环境变量获取默认值
var (
	dbTypeEnv = GetEnv("MONITOR_DB_TYPE", "sqlite")
	dbFileEnv = GetEnv("MONITOR_DB_FILE", "./data/monitor.db")
	dbHostEnv = GetEnv("MONITOR_DB_HOST", "localhost")
	dbPortEnv = GetEnv("MONITOR_DB_PORT", "3306")
	dbUserEnv = GetEnv("MONITOR_DB_USER", "root")
	dbPassEnv = GetEnv("MONITOR_DB_PASS", "")
	dbNameEnv = GetEnv("MONITOR_DB_NAME", "monitor")
)

var RootCmd = &cobra.Command{
	Use:   "Monitor",
	Short: "Monitor is a simple server monitoring tool",
	Long: `Monitor is a simple server monitoring tool.
Made by Akizon77 with love.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.SetArgs([]string{"server"})
		cmd.Execute()
	},
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	// 设置命令行参数，提供环境变量作为默认值
	RootCmd.PersistentFlags().StringVarP(&flags.DatabaseType, "db-type", "t", dbTypeEnv, "Database type (sqlite) [env: MONITOR_DB_TYPE]")
	RootCmd.PersistentFlags().StringVarP(&flags.DatabaseFile, "database", "d", dbFileEnv, "SQLite database file path [env: MONITOR_DB_FILE]")
	RootCmd.PersistentFlags().StringVar(&flags.DatabaseHost, "db-host", dbHostEnv, "Reserved database host parameter [env: MONITOR_DB_HOST]")
	RootCmd.PersistentFlags().StringVar(&flags.DatabasePort, "db-port", dbPortEnv, "Reserved database port parameter [env: MONITOR_DB_PORT]")
	RootCmd.PersistentFlags().StringVar(&flags.DatabaseUser, "db-user", dbUserEnv, "Reserved database username parameter [env: MONITOR_DB_USER]")
	RootCmd.PersistentFlags().StringVar(&flags.DatabasePass, "db-pass", dbPassEnv, "Reserved database password parameter [env: MONITOR_DB_PASS]")
	RootCmd.PersistentFlags().StringVar(&flags.DatabaseName, "db-name", dbNameEnv, "Reserved database name parameter [env: MONITOR_DB_NAME]")
	for _, name := range []string{"db-host", "db-port", "db-user", "db-pass", "db-name"} {
		_ = RootCmd.PersistentFlags().MarkHidden(name)
	}
}
