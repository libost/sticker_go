package constants

import (
	"path/filepath"
	"strings"
	"time"
)

var (
	Dir          string
	CacheDir     string
	ConfigFile   string
	LogDir       string
	DatabaseFile string
)

func init() {
	SetBaseDir("./")
}

func SetBaseDir(base string) {
	cleanBase := filepath.Clean(strings.TrimSpace(base))
	if cleanBase == "" || cleanBase == "." {
		Dir = "./"
	} else {
		Dir = filepath.ToSlash(cleanBase)
		if !strings.HasSuffix(Dir, "/") {
			Dir += "/"
		}
	}
	CacheDir = Dir + "cache/"
	ConfigFile = Dir + "config.yaml"
	LogDir = Dir + "logs/"
	DatabaseFile = Dir + "sticker_go.db"
}

const (
	RefundPeriod            = 7   // 退款周期，默认为7天
	DonationBonusMultiplier = 1.5 // 捐赠奖励倍数，默认为1.5倍
)

const (
	Owner   = "libost"
	Repo    = "sticker_go"
	RepoURL = "https://github.com/libost/sticker_go/releases/latest"
)

var (
	AcceptedPorts = []int{80, 443, 88, 8443} // 可接受的监听端口列表
)

type LogLevel struct {
	Level  string
	Number int
}

var (
	LogLevelDebug = LogLevel{"DEBUG", 0}
	LogLevelInfo  = LogLevel{"INFO", 1}
	LogLevelWarn  = LogLevel{"WARN", 2}
	LogLevelError = LogLevel{"ERROR", 3}
	LogLevelFatal = LogLevel{"FATAL", 4}
)

var DefaultConfig struct {
	General struct {
		Token        string `yaml:"token"`
		Limit        int    `yaml:"limit"`
		LimitPerPack int    `yaml:"limit_per_pack"`
		Adminkey     string `yaml:"adminkey"`
	} `yaml:"general,omitempty"`
	Cache struct {
		Enabled     bool `yaml:"enabled"`
		ExpireHours int  `yaml:"expire_hours"`
		SizeLimitMB int  `yaml:"size_limit_mb"`
	} `yaml:"cache,omitempty"`
	Subscription struct {
		Enabled bool   `yaml:"enabled"`
		Channel string `yaml:"channel,omitempty"`
	} `yaml:"subscription,omitempty"`
	Log struct {
		Level      string `yaml:"level"`
		ExpireDays int    `yaml:"expire_days"`
	} `yaml:"log,omitempty"`
	Webhook struct {
		Enabled      bool   `yaml:"enabled"`
		NginxEnabled bool   `yaml:"nginx_enabled"`
		URL          string `yaml:"url,omitempty"`
		Port         int    `yaml:"port"`
		Cert         struct {
			SelfSigned bool   `yaml:"self-signed"`
			CertPath   string `yaml:"cert_path,omitempty"`
			KeyPath    string `yaml:"key_path,omitempty"`
		} `yaml:"cert,omitempty"`
		Secret string `yaml:"secret,omitempty"`
	} `yaml:"webhook,omitempty"`
	Proxy struct {
		Enabled  bool   `yaml:"enabled"`
		Type     string `yaml:"type,omitempty"`
		Host     string `yaml:"host,omitempty"`
		Port     int    `yaml:"port,omitempty"`
		Username string `yaml:"username,omitempty"`
		Password string `yaml:"password,omitempty"`
	} `yaml:"proxy,omitempty"`
	Donation struct {
		Enabled        bool   `yaml:"enabled"`
		Title          string `yaml:"title"`
		Description    string `yaml:"description"`
		AmountRestrict struct {
			Min int `yaml:"min"`
			Max int `yaml:"max"`
		} `yaml:"amount_restrict,omitempty"`
	} `yaml:"donation,omitempty"`
	Misc struct {
		Timezone string `yaml:"timezone"`
	} `yaml:"misc,omitempty"`
}

func CurrentTime(timezone string) (string, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc, _ = time.LoadLocation("Asia/Shanghai") // Fallback to UTC+8 if timezone is invalid
	}
	return time.Now().In(loc).Format("2006-01-02 15:04:05"), err
}
