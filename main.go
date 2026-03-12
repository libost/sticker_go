package main

import (
	"fmt"
	"log"
	"net/http"
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

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	_ "time/tzdata"
)

func main() {
	// 1. 创建 Bot 实例
	cfg, err := config.Init()
	if err != nil {
		L.Log(fmt.Sprintf("failed to initialize config: %v", err), C.LogLevelFatal)
		log.Fatal(err)
	}
	if cfg == nil {
		L.Log("config initialization returned nil config", C.LogLevelFatal)
		log.Fatal("config initialization returned nil config")
	}
	if cfg.SubToggle && strings.TrimSpace(cfg.Channel) == "" {
		L.Log("subscription check is enabled but channel is not set in config", C.LogLevelFatal)
		log.Fatal("subscription check is enabled but channel is not set in config")
	}
	token := cfg.Token
	if strings.TrimSpace(token) == "" || token == "YOUR_TOKEN_HERE" {
		L.Log("config.yaml token is empty or still using the placeholder value", C.LogLevelFatal)
		log.Fatal("config.yaml token is empty or still using the placeholder value")
	}
	b, err := gotgbot.NewBot(token, &gotgbot.BotOpts{
		BotClient: &gotgbot.BaseBotClient{
			Client: http.Client{}, // 可以配置代理
		},
	})
	if err != nil {
		L.Log(fmt.Sprintf("failed to create bot: %v", err), C.LogLevelFatal)
		log.Fatal("failed to create bot")
	}
	_, err = database.Init("init", 0, nil) // 初始化数据库连接
	if err != nil {
		L.Log(fmt.Sprintf("failed to initialize database: %v", err), C.LogLevelFatal)
		log.Fatal("failed to initialize database")
	}
	cacheDir := C.CacheDir
	cacheInfo, err := os.Stat(cacheDir) // 检查缓存目录是否存在
	if os.IsNotExist(err) || (err == nil && !cacheInfo.IsDir()) {
		err = os.Mkdir(cacheDir, 0755)
		if err != nil {
			L.Log("failed to create cache directory", C.LogLevelFatal)
			log.Fatal("failed to create cache directory: " + err.Error())
		}
	}
	// 定时清理缓存目录中的过期文件
	go func() {
		// 立即执行一次
		cleanCache(cacheDir, cfg)

		// 之后每隔 1 小时检查一次
		ticker := time.NewTicker(time.Duration(cfg.CacheExpireHours) * time.Hour)
		for range ticker.C {
			cleanCache(cacheDir, cfg)
		}
	}()
	cmd := exec.Command("ffmpeg", "-version")
	err = cmd.Run() // 检查 ffmpeg 是否可用
	if err != nil {
		L.Log(fmt.Sprintf("ffmpeg is not installed or not in PATH: %v", err), C.LogLevelFatal)
		log.Fatal("ffmpeg is not installed or not in PATH")
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
	go func() {
		// 立即执行一次
		clearLogs(logDir, cfg)
		// 之后每天检查一次
		ticker := time.NewTicker(24 * time.Hour)
		for range ticker.C {
			clearLogs(logDir, cfg)
		}
	}()

	// 2. 创建分发器 (Dispatcher)
	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{
		Error: func(b *gotgbot.Bot, ctx *ext.Context, err error) ext.DispatcherAction {
			L.Log(fmt.Sprintf("发生错误: %v", err), C.LogLevelError)
			return ext.DispatcherActionNoop
		},
		MaxRoutines: ext.DefaultMaxRoutines,
	})

	// 3. 创建更新器 (Updater) 关联分发器
	updater := ext.NewUpdater(dispatcher, nil)

	// 4. 添加处理器 (Handler)
	commands.AddHandlers(dispatcher)
	stickers.AddHandlers(dispatcher)
	callback.AddHandlers(dispatcher)

	// 5. 启动轮询
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
	logText := fmt.Sprintf("%s has started...", b.User.Username)
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
	threshold := time.Duration(cfg.CacheExpireHours) * time.Hour // 设定过期时间为配置中指定的小时数

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
	if totalSize > int64(cfg.CacheSizeLimitMB)*1024*1024 {
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
	threshold := time.Duration(cfg.LogExpireDays) * 24 * time.Hour // 设定过期时间为配置中指定的天数
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
