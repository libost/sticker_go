package utils

import (
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	C "github.com/libost/sticker_go/constants"
	DB "github.com/libost/sticker_go/database"
	"github.com/libost/sticker_go/log"

	"github.com/goccy/go-yaml"
)

var (
	CurrentTasks = int64(0)
	startTime    = time.Now()
)

func AddTaskCount() error {
	CurrentTasks++
	if CurrentTasks < 0 {
		CurrentTasks = 0
		return errors.New("task count cannot be negative")
	}
	return nil
}

func SubtractTaskCount() error {
	CurrentTasks--
	if CurrentTasks < 0 {
		CurrentTasks = 0
		return errors.New("task count cannot be negative")
	}
	return nil
}

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

func ActiveGC() {
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			if CurrentTasks == 0 {
				log.Log("No active tasks, running garbage collection...", C.LogLevelDebug)
				runtime.GC()
			} else {
				log.Log("Skipping GC as tasks are still running. CurrentTasks count: "+fmt.Sprint(CurrentTasks), C.LogLevelInfo)
				continue
			}
		}
	}()
}

// Returns the uptime of the application in seconds, minutes, hours, and days.
func Uptime() (float64, int64, int64, int64) {
	uptime := time.Since(startTime).Seconds()
	if uptime >= 60 {
		minutes := int64(uptime / 60)
		seconds := math.Mod(uptime, 60)
		if minutes >= 60 {
			hours := minutes / 60
			minutes = int64(minutes % 60)
			if hours >= 24 {
				days := hours / 24
				hours = int64(hours % 24)
				return seconds, minutes, hours, days
			}
			return seconds, minutes, hours, 0
		}
		return seconds, minutes, 0, 0
	}
	return time.Since(startTime).Seconds(), 0, 0, 0
}

func refreshUsage() {
	log.Log("Refreshing usage counter...", C.LogLevelDebug)
	_, err := DB.Init("refreshUsageCounter", 0, nil)
	if err != nil {
		log.Log("Failed to refresh usage counter: "+err.Error(), C.LogLevelError)
	} else {
		log.Log("Usage counter refreshed successfully.", C.LogLevelInfo)
	}
}

func RefreshUsageCounter() {
	var complete bool
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		refreshUsage() // Initial refresh on startup
		for range ticker.C {
			if CurrentTasks == 0 {
				refreshUsage()
				complete = true
			} else {
				log.Log("Postponing usage counter refresh as tasks are still running. CurrentTasks count: "+fmt.Sprint(CurrentTasks), C.LogLevelInfo)
				for range 5 {
					time.Sleep(30 * time.Minute)
					if CurrentTasks != 0 {
						log.Log("Still active tasks, postponing usage counter refresh. CurrentTasks count: "+fmt.Sprint(CurrentTasks), C.LogLevelInfo)
						continue
					}
					log.Log("Refreshing usage counter after waiting...", C.LogLevelDebug)
					refreshUsage()
					complete = true
					break
				}
			}
			if !complete {
				log.Log("Unable to refresh usage counter after multiple attempts due to active tasks. Waiting for next cycle...", C.LogLevelError)
			}
			complete = false
		}
	}()
}
