package utils

import (
	"errors"
	"fmt"
	"net"
	"net/http"
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

func HealthCheckEP() (*http.Server, <-chan error, error) {
	// Create a simple HTTP server that listens on port 3417 and responds with "OK" to health check requests.
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	srv := &http.Server{
		Addr:    ":3417",
		Handler: mux,
	}

	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to listen on %s: %w", srv.Addr, err)
	}

	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)
		if serveErr := srv.Serve(ln); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
		}
	}()

	return srv, errCh, nil
}
