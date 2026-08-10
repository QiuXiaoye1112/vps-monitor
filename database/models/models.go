package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// Client represents a registered client device
type Client struct {
	UUID                string    `json:"uuid,omitempty" gorm:"type:varchar(36);primaryKey"`
	Token               string    `json:"token,omitempty" gorm:"type:varchar(255);unique;not null"`
	Name                string    `json:"name" gorm:"type:varchar(100)"`
	CpuName             string    `json:"cpu_name" gorm:"type:varchar(100)"`
	Virtualization      string    `json:"virtualization" gorm:"type:varchar(50)"`
	Arch                string    `json:"arch" gorm:"type:varchar(50)"`
	CpuCores            int       `json:"cpu_cores" gorm:"type:int"`
	CpuPhysicalCores    int       `json:"cpu_physical_cores" gorm:"type:int"`
	OS                  string    `json:"os" gorm:"type:varchar(100)"`
	KernelVersion       string    `json:"kernel_version" gorm:"type:varchar(100)"`
	IPv4                string    `json:"ipv4,omitempty" gorm:"type:varchar(100)"`
	IPv6                string    `json:"ipv6,omitempty" gorm:"type:varchar(100)"`
	Region              string    `json:"region" gorm:"type:varchar(100)"`
	Remark              string    `json:"remark,omitempty" gorm:"type:longtext"`
	PublicRemark        string    `json:"public_remark,omitempty" gorm:"type:longtext"`
	MemTotal            int64     `json:"mem_total" gorm:"type:bigint"`
	SwapTotal           int64     `json:"swap_total" gorm:"type:bigint"`
	DiskTotal           int64     `json:"disk_total" gorm:"type:bigint"`
	Version             string    `json:"version,omitempty" gorm:"type:varchar(100)"`
	Weight              int       `json:"weight" gorm:"type:int"`
	Price               float64   `json:"price"`
	BillingCycle        int       `json:"billing_cycle"`
	Currency            string    `json:"currency" gorm:"type:varchar(20);default:'$'"`
	ExpiredAt           LocalTime `json:"expired_at" gorm:"type:timestamp"`
	Group               string    `json:"group" gorm:"type:varchar(100)"`
	Tags                string    `json:"tags" gorm:"type:text"` // split by ';'
	Hidden              bool      `json:"hidden" gorm:"default:false"`
	TrafficLimit        int64     `json:"traffic_limit" gorm:"type:bigint"`
	TrafficLimitType    string    `json:"traffic_limit_type" gorm:"type:varchar(10);default:'max'"` // 流量阈值类型：sum max min up down
	TrafficResetDay     int       `json:"traffic_reset_day" gorm:"type:int;default:1"`
	TrafficResetHour    int       `json:"traffic_reset_hour" gorm:"type:int;default:0"`
	TrafficResetMinute  int       `json:"traffic_reset_minute" gorm:"type:int;default:0"`
	TrafficResetEnabled bool      `json:"traffic_reset_enabled" gorm:"type:boolean;default:true"`
	TrafficComp         int64     `json:"traffic_compensation" gorm:"column:traffic_compensation;type:bigint;default:0"`
	TrafficCarry        int64     `json:"-" gorm:"column:traffic_carry;type:bigint;default:0"` // Legacy aggregate; migrated into the directional fields below.
	TrafficCarryUp      int64     `json:"-" gorm:"column:traffic_carry_up;type:bigint;default:0"`
	TrafficCarryDown    int64     `json:"-" gorm:"column:traffic_carry_down;type:bigint;default:0"`
	TrafficCompResetAt  LocalTime `json:"traffic_compensation_reset_at" gorm:"column:traffic_compensation_reset_at"`
	TrafficClearedAt    LocalTime `json:"traffic_cleared_at" gorm:"column:traffic_cleared_at"`
	TrafficBaselineUp   int64     `json:"-" gorm:"column:traffic_baseline_up;type:bigint;default:0"`
	TrafficBaselineDown int64     `json:"-" gorm:"column:traffic_baseline_down;type:bigint;default:0"`
	PingTaskOrder       UintArray `json:"ping_task_order" gorm:"type:longtext"`
	CreatedAt           LocalTime `json:"created_at"`
	UpdatedAt           LocalTime `json:"updated_at"`
	LastReportAt        LocalTime `json:"last_report_at,omitempty"`
}

// User represents an authenticated user
type User struct {
	UUID      string    `json:"uuid,omitempty" gorm:"type:varchar(36);primaryKey"`
	Username  string    `json:"username" gorm:"type:varchar(50);unique;not null"`
	Passwd    string    `json:"passwd,omitempty" gorm:"type:varchar(255);not null"` // Hashed password
	SSOType   string    `json:"sso_type" gorm:"type:varchar(20)"`                   // e.g., "github", "google"
	SSOID     string    `json:"sso_id" gorm:"type:varchar(100)"`                    // OAuth provider's user ID
	TwoFactor string    `json:"two_factor,omitempty" gorm:"type:varchar(255)"`      // 2FA secret
	Sessions  []Session `json:"sessions,omitempty" gorm:"foreignKey:UUID;references:UUID;constraint:OnDelete:CASCADE,OnUpdate:CASCADE"`
	CreatedAt LocalTime `json:"created_at"`
	UpdatedAt LocalTime `json:"updated_at"`
}

// Session manages user sessions
type Session struct {
	UUID            string    `json:"uuid" gorm:"type:varchar(36)"`
	Session         string    `json:"session" gorm:"type:varchar(255);primaryKey;uniqueIndex:idx_sessions_session;not null"`
	UserAgent       string    `json:"user_agent" gorm:"type:text"`
	Ip              string    `json:"ip" gorm:"type:varchar(100)"`
	LoginMethod     string    `json:"login_method" gorm:"type:varchar(50)"`
	LatestOnline    LocalTime `json:"latest_online" gorm:"type:timestamp"`
	LatestUserAgent string    `json:"latest_user_agent" gorm:"type:text"`
	LatestIp        string    `json:"latest_ip" gorm:"type:varchar(100)"`
	Expires         LocalTime `json:"expires" gorm:"not null"`
	CreatedAt       LocalTime `json:"created_at"`
}

// Record logs client metrics over time
type Record struct {
	Client         string    `json:"client" gorm:"type:varchar(36);index"`
	Time           LocalTime `json:"time" gorm:"index"`
	Cpu            float32   `json:"cpu" gorm:"type:decimal(5,2)"` // e.g., 75.50%
	Ram            int64     `json:"ram" gorm:"type:bigint"`
	RamTotal       int64     `json:"ram_total" gorm:"type:bigint"`
	Swap           int64     `json:"swap" gorm:"type:bigint"`
	SwapTotal      int64     `json:"swap_total" gorm:"type:bigint"`
	Load           float32   `json:"load" gorm:"type:decimal(5,2)"`
	Temp           float32   `json:"temp" gorm:"type:decimal(5,2)"`
	Disk           int64     `json:"disk" gorm:"type:bigint"`
	DiskTotal      int64     `json:"disk_total" gorm:"type:bigint"`
	NetIn          int64     `json:"net_in" gorm:"type:bigint"`
	NetOut         int64     `json:"net_out" gorm:"type:bigint"`
	NetTotalUp     int64     `json:"net_total_up" gorm:"type:bigint"`
	NetTotalDown   int64     `json:"net_total_down" gorm:"type:bigint"`
	TrafficUp      int64     `json:"traffic_up" gorm:"type:bigint"`
	TrafficDown    int64     `json:"traffic_down" gorm:"type:bigint"`
	Process        int       `json:"process"`
	Connections    int       `json:"connections"`
	ConnectionsUdp int       `json:"connections_udp"`
	//Uptime         int64     `json:"uptime" gorm:"type:bigint"`
}

// StringArray represents a slice of strings stored as JSON in the database
// StringArray 存储为 JSON 的字符串切片类型
type StringArray []string

func (sa *StringArray) Scan(value interface{}) error {
	var bytes []byte
	switch v := value.(type) {
	case nil:
		*sa = StringArray{}
		return nil
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("failed to scan StringArray: unsupported value type %T", value)
	}
	if len(bytes) == 0 {
		*sa = StringArray{}
		return nil
	}
	return json.Unmarshal(bytes, sa)
}

func (sa StringArray) Value() (driver.Value, error) {
	return json.Marshal(sa)
}

// UintArray represents an ordered list of unsigned integer IDs stored as JSON.
type UintArray []uint

func (ua *UintArray) Scan(value interface{}) error {
	var bytes []byte
	switch v := value.(type) {
	case nil:
		*ua = UintArray{}
		return nil
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("failed to scan UintArray: unsupported value type %T", value)
	}
	if len(bytes) == 0 {
		*ua = UintArray{}
		return nil
	}
	return json.Unmarshal(bytes, ua)
}

func (ua UintArray) Value() (driver.Value, error) {
	if ua == nil {
		ua = UintArray{}
	}
	return json.Marshal(ua)
}
