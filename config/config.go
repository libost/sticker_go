package config

import (
	"os"

	C "github.com/libost/sticker_go/constants"

	"github.com/creasty/defaults"
	"github.com/goccy/go-yaml"
)

type Config struct {
	General struct {
		Token        string `yaml:"token"`
		Limit        int    `yaml:"limit" default:"100"`
		LimitPerPack int    `yaml:"limit_per_pack" default:"100"`
		Adminkey     string `yaml:"adminkey"`
		TgsSupport   bool   `yaml:"tgs_support" default:"false"`
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
		Cert         struct {
			SelfSigned bool   `yaml:"self-signed" default:"false"`
			CertPath   string `yaml:"cert_path,omitempty"`
			KeyPath    string `yaml:"key_path,omitempty"`
		} `yaml:"cert,omitempty"`
		Secret string `yaml:"secret,omitempty"`
	} `yaml:"webhook,omitempty"`
	Proxy struct {
		Enabled  bool   `yaml:"enabled" default:"false"`
		Type     string `yaml:"type,omitempty"`
		Host     string `yaml:"host,omitempty"`
		Port     int    `yaml:"port,omitempty"`
		Username string `yaml:"username,omitempty"`
		Password string `yaml:"password,omitempty"`
	} `yaml:"proxy,omitempty"`
	Donation struct {
		Enabled        bool   `yaml:"enabled" default:"false"`
		BonusEnabled   bool   `yaml:"bonus_enabled" default:"false"`
		Title          string `yaml:"title" default:"支持开发"`
		Description    string `yaml:"description" default:"如果你喜欢这个项目，欢迎通过以下方式支持开发！"`
		AmountRestrict struct {
			Min int `yaml:"min" default:"1"`
			Max int `yaml:"max" default:"10000"`
		} `yaml:"amount_restrict,omitempty"`
	} `yaml:"donation,omitempty"`
	Misc struct {
		Timezone string `yaml:"timezone" default:"Asia/Shanghai"`
		//SelfUse  bool   `yaml:"self_use" default:"false"`
	} `yaml:"misc,omitempty"`
}

var AppConfig *Config

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
	if cf.General.Limit <= 0 {
		cf.General.Limit = 100
	}
	if cf.General.LimitPerPack <= 0 {
		cf.General.LimitPerPack = 100
	}
	if cf.Cache.ExpireHours <= 0 && cf.Cache.Enabled {
		cf.Cache.ExpireHours = 1
	}
	if cf.Cache.SizeLimitMB <= 0 && cf.Cache.Enabled {
		cf.Cache.SizeLimitMB = 500
	}
	if cf.Log.ExpireDays <= 0 {
		cf.Log.ExpireDays = 7
	}
	return cf, nil
}

func Init() {
	var configPath = C.ConfigFile
	_, err := os.Stat(configPath)
	if os.IsNotExist(err) {
		panic(err)
	}
	cf, err := loadConfig(configPath)
	if err != nil {
		panic(err)
	}
	AppConfig = cf
}
