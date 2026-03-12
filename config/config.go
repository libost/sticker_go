package config

import (
	"os"

	C "libost/sticker_go/constants"

	"github.com/goccy/go-yaml"
)

type Config struct {
	Token            string `yaml:"token"`
	Limit            int    `yaml:"limit"`
	LimitPerPack     int    `yaml:"limit_per_pack"`
	CacheExpireHours int    `yaml:"cache_expire_hours"`
	CacheSizeLimitMB int    `yaml:"cache_size_limit_mb"`
	Adminkey         string `yaml:"adminkey"`
}

func loadConfig(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	cf := &Config{}
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
