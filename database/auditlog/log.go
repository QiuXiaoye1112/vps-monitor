package auditlog

import (
	"log"
	"time"

	"github.com/monitor-monitor/monitor/database/dbcore"
	"github.com/monitor-monitor/monitor/database/models"
	"gorm.io/gorm"
)

func Log(ip, uuid, message, msgType string) {
	now := time.Now()
	db := dbcore.GetDBInstance()
	logEntry := &models.Log{
		IP:      ip,
		UUID:    uuid,
		Message: message,
		MsgType: msgType,
		Time:    models.FromTime(now),
	}
	db.Create(logEntry)
}

func EventLog(eventType, message string) {
	Log("", "", message, eventType)
}

// Delete logs older than 30 days
func RemoveOldLogs() {
	if err := removeOldLogs(dbcore.GetDBInstance(), time.Now()); err != nil {
		log.Println("Failed to remove old logs:", err)
	}
}

func removeOldLogs(db *gorm.DB, now time.Time) error {
	threshold := now.AddDate(0, 0, -30)
	return db.Where("time < ?", models.FromTime(threshold)).Delete(&models.Log{}).Error
}
