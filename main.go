package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"libost/sticker_go/callback"
	"libost/sticker_go/commands"
	"libost/sticker_go/config"
	C "libost/sticker_go/constants"
	"libost/sticker_go/database"
	L "libost/sticker_go/log"
	"libost/sticker_go/stickers"
	"libost/sticker_go/utils"
	V "libost/sticker_go/version"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"golang.org/x/net/proxy"

	_ "time/tzdata"
)

func main() {
	args := os.Args
	if len(args) > 1 && (args[1] == "version" || args[1] == "-v" || args[1] == "--version") {
		fmt.Printf("Sticker Bot Version: %s\nBuild Time: %s\n", V.Version, V.BuildTime)
		return
	}
	if _, err := os.Stat("config.yaml"); os.IsNotExist(err) {
		L.Log("config.yaml not found, creating default config.yaml", C.LogLevelInfo)
		_ = utils.ConfigToYAML()
		L.Log("config.yaml not found, a default config.yaml has been created. Please edit it and restart the bot.", C.LogLevelFatal)
	}
	cfg, err := config.Init()
	if err != nil {
		L.Log(fmt.Sprintf("failed to initialize config: %v", err), C.LogLevelFatal)
	}
	if cfg == nil {
		L.Log("config initialization returned nil config", C.LogLevelFatal)
	}
	if cfg.Subscription.Enabled && strings.TrimSpace(cfg.Subscription.Channel) == "" {
		L.Log("subscription check is enabled but channel is not set in config", C.LogLevelFatal)
	}
	token := cfg.General.Token
	if strings.TrimSpace(token) == "" || token == "YOUR_TOKEN_HERE" {
		L.Log("config.yaml token is empty or still using the placeholder value", C.LogLevelFatal)
	}
	httpClient := &http.Client{
		Timeout: time.Second * 10,
	}
	if cfg.Proxy.Enabled {
		switch cfg.Proxy.Type {
		case "socks5":
			dialer, _ := proxy.SOCKS5("tcp", fmt.Sprintf("%s:%d", cfg.Proxy.Host, cfg.Proxy.Port), nil, proxy.Direct)
			httpClient = &http.Client{
				Transport: &http.Transport{
					Dial: dialer.Dial,
				},
			}
		case "http":
			proxyUrl, _ := url.Parse(fmt.Sprintf("http://%s:%d", cfg.Proxy.Host, cfg.Proxy.Port))
			httpClient = &http.Client{
				Transport: &http.Transport{
					Proxy: http.ProxyURL(proxyUrl),
				},
			}
		}
	}
	b, err := gotgbot.NewBot(token, &gotgbot.BotOpts{
		BotClient: &gotgbot.BaseBotClient{
			Client: *httpClient, // 可以配置代理
		},
	})
	if err != nil {
		L.Log(fmt.Sprintf("failed to create bot: %v", err), C.LogLevelFatal)
	}
	_, err = database.Init("init", 0, nil) // 初始化数据库连接
	if err != nil {
		L.Log(fmt.Sprintf("failed to initialize database: %v", err), C.LogLevelFatal)
	}
	cacheDir := C.CacheDir
	cacheInfo, err := os.Stat(cacheDir) // 检查缓存目录是否存在
	if os.IsNotExist(err) || (err == nil && !cacheInfo.IsDir()) {
		err = os.Mkdir(cacheDir, 0755)
		if err != nil {
			L.Log("failed to create cache directory", C.LogLevelFatal)
		}
	}
	// 定时清理缓存目录中的过期文件
	go func() {
		// 立即执行一次
		cleanCache(cacheDir, cfg)

		// 之后每隔 1 小时检查一次
		ticker := time.NewTicker(time.Duration(cfg.Cache.ExpireHours) * time.Hour)
		for range ticker.C {
			cleanCache(cacheDir, cfg)
		}
	}()
	cmd := exec.Command("ffmpeg", "-version")
	err = cmd.Run() // 检查 ffmpeg 是否可用
	if err != nil {
		L.Log(fmt.Sprintf("ffmpeg is not installed or not in PATH: %v", err), C.LogLevelFatal)
	}
	if cfg.General.TgsSupport {
		cmd = exec.Command("docker", "images", "-q", "edasriyan/lottie-to-gif")
		output, err := cmd.Output() // 检查 Docker 镜像是否可用
		if err != nil {
			L.Log(fmt.Sprintf("Docker is not installed or 'edasriyan/lottie-to-gif' image is not available: %v", err), C.LogLevelFatal)
		}
		if len(strings.TrimSpace(string(output))) == 0 {
			L.Log("Docker image 'edasriyan/lottie-to-gif' is not available", C.LogLevelFatal)
		}
	}
	logDir := C.LogDir
	logInfo, err := os.Stat(logDir) // 检查日志目录是否存在
	if os.IsNotExist(err) || (err == nil && !logInfo.IsDir()) {
		err = os.Mkdir(logDir, 0755)
		if err != nil {
			L.Log(fmt.Sprintf("failed to create log directory: %v", err), C.LogLevelFatal)
			log.Fatal("failed to create log directory")
		}
	}
	// 定时清理日志目录中的过期文件
	if cfg.Cache.Enabled {
		go func() {
			// 立即执行一次
			clearLogs(logDir, cfg)
			// 之后每天检查一次
			ticker := time.NewTicker(24 * time.Hour)
			for range ticker.C {
				clearLogs(logDir, cfg)
			}
		}()
	} else {
		L.Log("cache is disabled, cleanup cachedir", C.LogLevelInfo)
		_ = utils.RemoveDirContents(cacheDir)
	}

	// 创建分发器 (Dispatcher)
	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{
		Error: func(b *gotgbot.Bot, ctx *ext.Context, err error) ext.DispatcherAction {
			L.Log(fmt.Sprintf("发生错误: %v", err), C.LogLevelError)
			return ext.DispatcherActionNoop
		},
		MaxRoutines: ext.DefaultMaxRoutines,
	})

	// 创建更新器 (Updater) 关联分发器
	updater := ext.NewUpdater(dispatcher, nil)

	// 添加处理器 (Handler)
	commands.AddHandlers(dispatcher)
	stickers.AddHandlers(dispatcher)
	callback.AddHandlers(dispatcher)

	// 启动 Bot
	if cfg.Webhook.Enabled {
		webhookURL, err := url.Parse(cfg.Webhook.URL)
		if err != nil || webhookURL.Scheme == "" || webhookURL.Host == "" {
			L.Log(fmt.Sprintf("invalid webhook url: %q", cfg.Webhook.URL), C.LogLevelFatal)
		}
		if webhookURL.Path == "" || webhookURL.Path == "/" {
			L.Log("webhook url path is empty, please set a path like /webhook", C.LogLevelFatal)
		}

		// 启动 Webhook 服务器
		listenaddr := fmt.Sprintf("0.0.0.0:%d", cfg.Webhook.Port)
		if cfg.Webhook.NginxEnabled {
			// 如果启用了 Nginx 反向代理，监听本地回环地址
			listenaddr = fmt.Sprintf("127.0.0.1:%d", cfg.Webhook.Port)
		}
		webhookOpts := ext.WebhookOpts{
			ListenAddr:  listenaddr,         // 本地监听端口
			SecretToken: cfg.Webhook.Secret, // Webhook 密钥
		}
		err = updater.StartWebhook(b, webhookURL.Path, webhookOpts)
		if err != nil {
			L.Log(fmt.Sprintf("failed to start webhook: %v", err), C.LogLevelFatal)
			panic("failed to start webhook: " + err.Error())
		}

		// 设置 Telegram 的 Webhook URL
		_, err = b.SetWebhook(cfg.Webhook.URL, &gotgbot.SetWebhookOpts{
			SecretToken:        webhookOpts.SecretToken,
			DropPendingUpdates: true,
		})
		if err != nil {
			L.Log(fmt.Sprintf("failed to set webhook: %v", err), C.LogLevelFatal)
			panic("failed to set webhook: " + err.Error())
		}
	} else {
		err = updater.StartPolling(b, &ext.PollingOpts{
			DropPendingUpdates: true, // 启动时忽略之前的积压消息
			GetUpdatesOpts: &gotgbot.GetUpdatesOpts{
				Timeout: 9,
				RequestOpts: &gotgbot.RequestOpts{
					Timeout: time.Second * 10,
				},
			},
		})
		if err != nil {
			L.Log(fmt.Sprintf("failed to start polling: %v", err), C.LogLevelFatal)
			panic("failed to start polling: " + err.Error())
		}
	}
	var communicationMethod string
	if cfg.Webhook.Enabled {
		communicationMethod = "Webhook"
	} else {
		communicationMethod = "Polling"
	}
	logText := fmt.Sprintf("%s has started with %s enabled. Log Level: %s", b.User.Username, communicationMethod, cfg.Log.Level)
	L.Log(logText, C.LogLevelInfo)
	updater.Idle() // 阻塞直到进程被关闭
}

func cleanCache(cacheDir string, cfg *config.Config) {
	files, err := os.ReadDir(cacheDir)
	if err != nil {
		L.Log(fmt.Sprintf("failed to read cache directory: %v", err), C.LogLevelError)
		return
	}

	now := time.Now()
	threshold := time.Duration(cfg.Cache.ExpireHours) * time.Hour // 设定过期时间为配置中指定的小时数

	var totalSize int64

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		info, err := file.Info()
		if err != nil {
			continue
		}

		// 如果当前时间与文件修改时间的差值超过了阈值
		if now.Sub(info.ModTime()) > threshold {
			path := filepath.Join(cacheDir, file.Name())
			err := os.Remove(path)
			if err != nil {
				L.Log(fmt.Sprintf("failed to remove expired cache [%s]: %v", path, err), C.LogLevelError)
			} else {
				L.Log(fmt.Sprintf("removed expired cache: %s", file.Name()), C.LogLevelInfo)
			}
		}
		totalSize += info.Size()
	}
	if totalSize > int64(cfg.Cache.SizeLimitMB)*1024*1024 {
		L.Log(fmt.Sprintf("cache size exceeded limit: %d MB", totalSize/1024/1024), C.LogLevelWarn)
		err := os.RemoveAll(cacheDir)
		if err != nil {
			L.Log(fmt.Sprintf("failed to clean cache: %v", err), C.LogLevelError)
		} else {
			L.Log("all cache files have been cleaned", C.LogLevelInfo)
		}
		os.Mkdir(cacheDir, 0755) // 重新创建缓存目录
	}
}

func clearLogs(logDir string, cfg *config.Config) {
	files, err := os.ReadDir(logDir)
	if err != nil {
		L.Log(fmt.Sprintf("failed to read log directory: %v", err), C.LogLevelError)
		return
	}
	now := time.Now()
	threshold := time.Duration(cfg.Log.ExpireDays) * 24 * time.Hour // 设定过期时间为配置中指定的天数
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		info, err := file.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > threshold {
			path := filepath.Join(logDir, file.Name())
			err := os.Remove(path)
			if err != nil {
				L.Log(fmt.Sprintf("failed to remove expired log [%s]: %v", path, err), C.LogLevelError)
			} else {
				L.Log(fmt.Sprintf("removed expired log: %s", file.Name()), C.LogLevelInfo)
			}
		}
	}
}
