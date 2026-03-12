package config

import (
	"os"

	C "libost/sticker_go/constants"

	"github.com/creasty/defaults"
	"github.com/goccy/go-yaml"
)

type Config struct {
	Token            string `yaml:"token"`
	Limit            int    `yaml:"limit" default:"100"`
	LimitPerPack     int    `yaml:"limit_per_pack" default:"100"`
	CacheExpireHours int    `yaml:"cache_expire_hours" default:"1"`
	CacheSizeLimitMB int    `yaml:"cache_size_limit_mb" default:"500"`
	Adminkey         string `yaml:"adminkey" default:"123"`
	SubToggle        bool   `yaml:"sub_toggle" default:"false"`
	Channel          string `yaml:"channel,omitempty"`
	LogExpireDays    int    `yaml:"log_expire_days" default:"7"`
	Timezone         string `yaml:"timezone" default:"Asia/Shanghai"`
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
