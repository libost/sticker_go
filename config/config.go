package config

import (
	"os"

	C "libost/sticker_go/constants"

	"github.com/creasty/defaults"
	"github.com/goccy/go-yaml"
)

type Config struct {
	General struct {
		Token        string `yaml:"token"`
		Limit        int    `yaml:"limit" default:"100"`
		LimitPerPack int    `yaml:"limit_per_pack" default:"100"`
		Adminkey     string `yaml:"adminkey" default:"123"`
		TgsSupport   bool   `yaml:"tgs_support" default:"false"` // 是否支持 .tgs 动画贴纸，默认是 false，启用后会将 .tgs 动画贴纸转换为 GIF 格式发送，禁用后将直接发送原始 .tgs 文件
	} `yaml:"general,omitempty"`
	Cache struct {
		Enabled     bool `yaml:"enabled" default:"true"`
		ExpireHours int  `yaml:"expire_hours" default:"1"`
		SizeLimitMB int  `yaml:"size_limit_mb" default:"500"`
	} `yaml:"cache,omitempty"`
	Subscription struct {
		Enabled bool   `yaml:"enabled" default:"false"`
		Channel string `yaml:"channel,omitempty"`
	} `yaml:"subscription,omitempty"`
	Log struct {
		Level      string `yaml:"level" default:"INFO"`
		ExpireDays int    `yaml:"expire_days" default:"7"`
	} `yaml:"log,omitempty"`
	Webhook struct {
		Enabled      bool   `yaml:"enabled" default:"false"`
		NginxEnabled bool   `yaml:"nginx_enabled" default:"false"`
		URL          string `yaml:"url,omitempty"`
		Port         int    `yaml:"port" default:"8080"`
		Secret       string `yaml:"secret,omitempty"`
	} `yaml:"webhook,omitempty"`
	Proxy struct {
		Enabled  bool   `yaml:"enabled" default:"false"`
		Type     string `yaml:"type,omitempty"` // "socks5" 或 "http"
		Host     string `yaml:"host,omitempty"`
		Port     int    `yaml:"port,omitempty"`
		Username string `yaml:"username,omitempty"`
		Password string `yaml:"password,omitempty"`
	} `yaml:"proxy,omitempty"`
	Donation struct {
		Enabled        bool   `yaml:"enabled" default:"false"`
		Title          string `yaml:"title" default:"支持开发"`
		Description    string `yaml:"description" default:"如果你喜欢这个项目，欢迎通过以下方式支持开发！"`
		AmountRestrict struct {
			Min int `yaml:"min" default:"1"`
			Max int `yaml:"max" default:"10000"`
		} `yaml:"amount_restrict,omitempty"`
	} `yaml:"donation,omitempty"`
	Misc struct {
		Timezone string `yaml:"timezone" default:"Asia/Shanghai"`
	} `yaml:"misc,omitempty"`
}

func loadConfig(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	cf := &Config{}
	if err := defaults.Set(cf); err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(data, cf)
	if err != nil {
		return nil, err
	}
	return cf, nil
}

func Init() (*Config, error) {
	const configPath = C.ConfigFile
	_, err := os.Stat(configPath)
	if os.IsNotExist(err) {
		return nil, err
	}
	cf, err := loadConfig(configPath)
	if err != nil {
		return nil, err
	}
	return cf, nil
}
