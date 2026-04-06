package main

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/libost/sticker_go/callback"
	"github.com/libost/sticker_go/commands"
	"github.com/libost/sticker_go/config"
	C "github.com/libost/sticker_go/constants"
	"github.com/libost/sticker_go/database"
	L "github.com/libost/sticker_go/log"
	"github.com/libost/sticker_go/stickers"
	"github.com/libost/sticker_go/utils"
	V "github.com/libost/sticker_go/version"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/robfig/cron/v3"
	"golang.org/x/net/proxy"

	_ "time/tzdata"
)

func main() {
	// 捕获系统信号，在启动完成后执行优雅关闭流程
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigs)
	args := os.Args
	if len(args) > 1 {
		switch args[1] {
		case "version", "-v", "--version":
			fmt.Printf("Sticker Bot Version: %s\nBuild Time: %s\nGit Commit: %s\nBranch: %s", V.Version, V.BuildTime, V.GitCommit, V.Branch)
			return
		case "Dir", "-D", "-d":
			if len(args) < 3 || strings.TrimSpace(args[2]) == "" {
				L.Log("missing directory path for -d, usage: sticker_go -d <base_dir>", C.LogLevelFatal)
			}
			C.SetBaseDir(args[2])
			fmt.Printf("Base directory set to: %s\n", C.Dir)
		case "help", "-h", "--help":
			fmt.Printf("Usage: sticker_go [options]\n\nOptions:\n  version, -v, --version   Show version information\n  Dir, -D, -d <path>       Set base directory for data to be stored\n  help, -h, --help         Show this help message\n")
			return
		default:
			fmt.Printf("Unknown argument: %s\nUse 'sticker_go --help' to see available options.\n", args[1])
			return
		}
	}
	if _, err := os.Stat(C.ConfigFile); os.IsNotExist(err) {
		L.Log("config.yaml not found, creating default config.yaml", C.LogLevelInfo)
		_ = utils.ConfigToYAML()
		L.Log("config.yaml not found, a default config.yaml has been created. Please edit it and restart the bot.", C.LogLevelError)
		return
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
	switch "" {
	case strings.TrimSpace(token):
		L.Log("bot token is not set in config.yaml, please set your bot token to start the bot", C.LogLevelFatal)
		return
	case cfg.General.Adminkey:
		L.Log("admin key is not set in config.yaml, please set a random string as admin_key to protect your bot", C.LogLevelFatal)
		return
	}
	httpClient := httpClientWithProxy(cfg)
	b, err := gotgbot.NewBot(token, &gotgbot.BotOpts{
		BotClient: &gotgbot.BaseBotClient{
			Client: *httpClient,
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
	if cfg.Cache.Enabled {
		go func() {
			// 立即执行一次
			cleanFiles(cacheDir, cfg, "cache")

			// 之后每隔 1 小时检查一次
			ticker := time.NewTicker(time.Duration(cfg.Cache.ExpireHours) * time.Hour)
			for range ticker.C {
				cleanFiles(cacheDir, cfg, "cache")
			}
		}()
	} else {
		L.Log("cache is disabled in config, skipping cache cleanup routine", C.LogLevelInfo)
		os.RemoveAll(C.CacheDir)
		os.Mkdir(C.CacheDir, 0755) // 确保缓存目录存在但为空
	}
	preCheckPassed := preqrequisitesCheck(cfg)
	if !preCheckPassed {
		L.Log("prerequisites check failed, please fix the issues and restart the bot", C.LogLevelFatal)
		return
	}
	logDir := C.LogDir
	logInfo, err := os.Stat(logDir) // 检查日志目录是否存在
	if os.IsNotExist(err) || (err == nil && !logInfo.IsDir()) {
		err = os.Mkdir(logDir, 0755)
		if err != nil {
			L.Log(fmt.Sprintf("failed to create log directory: %v", err), C.LogLevelFatal)
		}
	}
	// 定时清理日志目录中的过期文件
	go func() {
		// 立即执行一次
		cleanFiles(logDir, cfg, "log")
		// 之后每天检查一次
		ticker := time.NewTicker(24 * time.Hour)
		for range ticker.C {
			cleanFiles(logDir, cfg, "log")
		}
	}()
	statsCleanup() // 启动统计数据清理任务
	// 创建分发器 (Dispatcher)
	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{
		Error: func(b *gotgbot.Bot, ctx *ext.Context, err error) ext.DispatcherAction {
			L.Log(fmt.Sprintf("Error occurred while processing update %v: %v", ctx.Update, err), C.LogLevelError)
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
	communicateCheck(cfg, updater, b)
	c := cron.New()
	go func() {
		sig := <-sigs
		L.Log(fmt.Sprintf("received signal: %v, starting graceful shutdown...", sig), C.LogLevelInfo)

		cronCtx := c.Stop()
		if cronCtx != nil {
			<-cronCtx.Done()
		}

		if err := updater.Stop(); err != nil {
			L.Log(fmt.Sprintf("graceful shutdown failed: %v", err), C.LogLevelError)
			return
		}

		L.Log("graceful shutdown completed", C.LogLevelInfo)
	}()

	updater.Idle() // 阻塞直到进程被关闭
}

func cleanFiles(dir string, cfg *config.Config, dirType string) {
	files, err := os.ReadDir(dir)
	if err != nil {
		L.Log(fmt.Sprintf("failed to read directory: %v", err), C.LogLevelError)
		return
	}

	now := time.Now()
	var threshold time.Duration
	switch dirType {
	case "cache":
		threshold = time.Duration(cfg.Cache.ExpireHours) * time.Hour // 设定过期时间为配置中指定的小时数
	case "log":
		threshold = time.Duration(cfg.Log.ExpireDays) * 24 * time.Hour // 设定过期时间为配置中指定的天数
	}

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
			path := filepath.Join(dir, file.Name())
			err := os.Remove(path)
			if err != nil {
				L.Log(fmt.Sprintf("failed to remove expired %s [%s]: %v", dirType, path, err), C.LogLevelError)
			} else {
				L.Log(fmt.Sprintf("removed expired %s: %s", dirType, file.Name()), C.LogLevelInfo)
			}
		}
		totalSize += info.Size()
	}
	if totalSize > int64(cfg.Cache.SizeLimitMB)*1024*1024 && dirType == "cache" {
		L.Log(fmt.Sprintf("cache size exceeded limit: %d MB", totalSize/1024/1024), C.LogLevelWarn)
		err := os.RemoveAll(dir)
		if err != nil {
			L.Log(fmt.Sprintf("failed to clean %s: %v", dirType, err), C.LogLevelError)
		} else {
			L.Log(fmt.Sprintf("all %s files have been cleaned", dirType), C.LogLevelInfo)
		}
		os.Mkdir(dir, 0755) // 重新创建缓存目录
	}
}

func isTelegramAcceptedWebhookPort(port int) bool {
	return slices.Contains(C.AcceptedPorts, port)
}

func httpClientWithProxy(cfg *config.Config) *http.Client {
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
			L.Log(fmt.Sprintf("using SOCKS5 proxy at %s:%d", cfg.Proxy.Host, cfg.Proxy.Port), C.LogLevelInfo)
		case "http":
			proxyUrl, _ := url.Parse(fmt.Sprintf("http://%s:%d", cfg.Proxy.Host, cfg.Proxy.Port))
			httpClient = &http.Client{
				Transport: &http.Transport{
					Proxy: http.ProxyURL(proxyUrl),
				},
			}
			L.Log(fmt.Sprintf("using HTTP proxy at %s:%d", cfg.Proxy.Host, cfg.Proxy.Port), C.LogLevelInfo)
		default:
			L.Log(fmt.Sprintf("unsupported proxy type: %s", cfg.Proxy.Type), C.LogLevelFatal)
		}
	}
	return httpClient
}

func preqrequisitesCheck(cfg *config.Config) bool {
	cmd := exec.Command("ffmpeg", "-version")
	err := cmd.Run()
	if err != nil {
		L.Log(fmt.Sprintf("ffmpeg is not installed or not in PATH: %v", err), C.LogLevelError)
		return false
	}
	if cfg.General.TgsSupport {
		cmd = exec.Command("docker", "images", "-q", "edasriyan/lottie-to-gif")
		output, err := cmd.Output()
		if err != nil {
			L.Log(fmt.Sprintf("Docker is not installed or 'edasriyan/lottie-to-gif' image is not available: %v", err), C.LogLevelError)
			return false
		}
		if len(strings.TrimSpace(string(output))) == 0 {
			L.Log("Docker image 'edasriyan/lottie-to-gif' is not available", C.LogLevelError)
			return false
		}
	}
	return true
}

func communicateCheck(cfg *config.Config, updater *ext.Updater, b *gotgbot.Bot) {
	communicationMethod := "Polling"
	if cfg.Webhook.Enabled {
		startWebhookCommunication(cfg, updater, b)
		communicationMethod = "Webhook"
	} else {
		startPollingCommunication(updater, b)
	}
	logText := fmt.Sprintf("%s has started with %s enabled. Log Level: %s", b.User.Username, communicationMethod, cfg.Log.Level)
	L.Log(logText, C.LogLevelInfo)
}

func startWebhookCommunication(cfg *config.Config, updater *ext.Updater, b *gotgbot.Bot) {
	webhookURL := mustParseWebhookURL(cfg.Webhook.URL)
	webhookOpts, setWebhookOpts := buildWebhookOptions(cfg)

	if err := updater.StartWebhook(b, webhookURL.Path, webhookOpts); err != nil {
		L.Log(fmt.Sprintf("failed to start webhook: %v", err), C.LogLevelFatal)
	}

	if _, err := b.SetWebhook(cfg.Webhook.URL, setWebhookOpts); err != nil {
		L.Log(fmt.Sprintf("failed to set webhook: %v", err), C.LogLevelFatal)
	}
}

func mustParseWebhookURL(rawURL string) *url.URL {
	webhookURL, err := url.Parse(rawURL)
	if err != nil || webhookURL.Scheme == "" || webhookURL.Host == "" {
		L.Log(fmt.Sprintf("invalid webhook url: %q", rawURL), C.LogLevelFatal)
	}
	if webhookURL.Path == "" || webhookURL.Path == "/" {
		L.Log("webhook url path is empty, please set a path like /webhook", C.LogLevelFatal)
	}
	return webhookURL
}

func buildWebhookOptions(cfg *config.Config) (ext.WebhookOpts, *gotgbot.SetWebhookOpts) {
	listenAddr := fmt.Sprintf("0.0.0.0:%d", cfg.Webhook.Port)
	webhookOpts := ext.WebhookOpts{
		ListenAddr:  listenAddr,
		SecretToken: cfg.Webhook.Secret,
	}
	setWebhookOpts := &gotgbot.SetWebhookOpts{
		SecretToken:        webhookOpts.SecretToken,
		DropPendingUpdates: true,
	}

	if cfg.Webhook.NginxEnabled {
		webhookOpts.ListenAddr = fmt.Sprintf("127.0.0.1:%d", cfg.Webhook.Port)
		return webhookOpts, setWebhookOpts
	}

	applyWebhookTLSConfig(cfg, &webhookOpts, setWebhookOpts)
	return webhookOpts, setWebhookOpts
}

func applyWebhookTLSConfig(cfg *config.Config, webhookOpts *ext.WebhookOpts, setWebhookOpts *gotgbot.SetWebhookOpts) {
	if !isTelegramAcceptedWebhookPort(cfg.Webhook.Port) {
		L.Log(fmt.Sprintf("Webhook port %d is not accepted by Telegram API, please consider using a standard port or nginx reverse proxy, this bot will quit.", cfg.Webhook.Port), C.LogLevelFatal)
	}
	if cfg.Webhook.Cert.CertPath == "" || cfg.Webhook.Cert.KeyPath == "" {
		L.Log("SSL cert or key path is not set, webhook will be started without TLS which is not secure and may not work with Telegram API, please consider setting cert_path and key_path in config.yaml or using nginx reverse proxy, this bot will quit.", C.LogLevelFatal)
	}

	webhookOpts.CertFile = cfg.Webhook.Cert.CertPath
	webhookOpts.KeyFile = cfg.Webhook.Cert.KeyPath
	if cfg.Webhook.Cert.SelfSigned {
		certFile, err := os.Open(cfg.Webhook.Cert.CertPath)
		if err != nil {
			L.Log(fmt.Sprintf("failed to open SSL certificate file: %v", err), C.LogLevelFatal)
		}
		defer certFile.Close()
		setWebhookOpts.Certificate = gotgbot.InputFileByReader(filepath.Base(cfg.Webhook.Cert.CertPath), certFile)
		L.Log("self-signed certificate is enabled, certificate will be uploaded to Telegram when setting webhook", C.LogLevelInfo)
	}
}

func startPollingCommunication(updater *ext.Updater, b *gotgbot.Bot) {
	if _, err := b.DeleteWebhook(&gotgbot.DeleteWebhookOpts{
		DropPendingUpdates: true,
	}); err != nil {
		L.Log(fmt.Sprintf("failed to delete existing webhook: %v", err), C.LogLevelWarn)
	}

	err := updater.StartPolling(b, &ext.PollingOpts{
		DropPendingUpdates: true,
		GetUpdatesOpts: &gotgbot.GetUpdatesOpts{
			Timeout: 9,
			RequestOpts: &gotgbot.RequestOpts{
				Timeout: time.Second * 10,
			},
		},
	})
	if err != nil {
		L.Log(fmt.Sprintf("failed to start polling: %v", err), C.LogLevelFatal)
	}
}

func statsCleanup() {
	last_clean_time, err := database.Init("getLastCleanupTime", 0, nil) // 获取上次清理每周数据的时间
	if err != nil {
		L.Log(fmt.Sprintf("failed to get last clean time from database: %v", err), C.LogLevelFatal)
	}
	lastCleanupAt := int64(0)
	if v, ok := last_clean_time["last_cleanup_at"]; ok {
		switch ts := v.(type) {
		case float64:
			lastCleanupAt = int64(ts)
		case int64:
			lastCleanupAt = ts
		case int:
			lastCleanupAt = int64(ts)
		default:
			L.Log(fmt.Sprintf("invalid last_cleanup_at type: %T, fallback to 0", v), C.LogLevelWarn)
		}
	} else {
		L.Log("last_cleanup_at is missing, fallback to 0", C.LogLevelWarn)
	}
	if lastCleanupAt+int64(7*24*time.Hour.Seconds()) < time.Now().Unix() { // 如果上次清理时间超过7天，立即清理一次每周数据
		_, err := database.Init("clearWeeklyStats", 0, nil)
		if err != nil {
			L.Log(fmt.Sprintf("failed to clear weekly stats: %v", err), C.LogLevelFatal)
		}
		L.Log("weekly stats have been cleared on startup", C.LogLevelInfo)
	}
	c := cron.New()
	// 每周一凌晨 0 点清理一次数据库中的每周统计数据
	_, err = c.AddFunc("0 0 * * 1", func() {
		_, err := database.Init("clearWeeklyStats", 0, nil)
		if err != nil {
			L.Log(fmt.Sprintf("failed to clear weekly stats: %v", err), C.LogLevelFatal)
		} else {
			L.Log("weekly stats have been cleared by scheduled task", C.LogLevelInfo)
		}
	})
	if err != nil {
		L.Log(fmt.Sprintf("failed to schedule weekly stats clearing: %v", err), C.LogLevelFatal)
	} else {
		c.Start()
	}
}
