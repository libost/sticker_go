package constants

import "time"

const (
	CacheDir   = "./cache/"
	ConfigFile = "./config.yaml"
	LogDir     = "./logs/"
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
		Secret       string `yaml:"secret,omitempty"`
	} `yaml:"webhook,omitempty"`
	Proxy struct {
		Enabled  bool   `yaml:"enabled"`
		Type     string `yaml:"type,omitempty"`
		Host     string `yaml:"host,omitempty"`
		Port     int    `yaml:"port,omitempty"`
		Username string `yaml:"username,omitempty"`
		Password string `yaml:"password,omitempty"`
	} `yaml:"proxy,omitempty"`
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
