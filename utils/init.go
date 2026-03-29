package utils

import (
	"fmt"
	"os"
	"path/filepath"

	C "github.com/libost/sticker_go/constants"

	"github.com/goccy/go-yaml"
)

func ConfigToYAML() error {
	defaultConfig := C.DefaultConfig
	defaultConfig.General.Limit = 100
	defaultConfig.General.LimitPerPack = 100
	defaultConfig.Cache.Enabled = true
	defaultConfig.Cache.ExpireHours = 1
	defaultConfig.Cache.SizeLimitMB = 500
	defaultConfig.Subscription.Enabled = false
	defaultConfig.Subscription.Channel = ""
	defaultConfig.Log.Level = "INFO"
	defaultConfig.Log.ExpireDays = 7
	defaultConfig.Subscription.Enabled = false
	defaultConfig.Subscription.Channel = ""
	defaultConfig.Webhook.Enabled = false
	defaultConfig.Webhook.NginxEnabled = false
	defaultConfig.Webhook.URL = ""
	defaultConfig.Webhook.Port = 8080
	defaultConfig.Webhook.Secret = ""
	defaultConfig.Proxy.Enabled = false
	defaultConfig.Proxy.Type = ""
	defaultConfig.Proxy.Host = ""
	defaultConfig.Proxy.Port = 0
	defaultConfig.Proxy.Username = ""
	defaultConfig.Proxy.Password = ""
	defaultConfig.Donation.Enabled = false
	defaultConfig.Donation.BonusEnabled = false
	defaultConfig.Donation.Title = "支持开发"
	defaultConfig.Donation.Description = "如果你喜欢这个项目，欢迎通过以下方式支持开发！"
	defaultConfig.Donation.AmountRestrict.Min = 1
	defaultConfig.Donation.AmountRestrict.Max = 10000
	defaultConfig.Misc.Timezone = "Asia/Shanghai"

	yamlBytes, err := yaml.Marshal(defaultConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal config to YAML: %v", err)
	}
	err = os.MkdirAll(filepath.Dir(C.ConfigFile), 0755)
	if err != nil {
		return fmt.Errorf("failed to create config directory: %v", err)
	}
	err = os.WriteFile(C.ConfigFile, yamlBytes, 0644)
	if err != nil {
		return fmt.Errorf("failed to write config to file: %v", err)
	}
	return nil
}
