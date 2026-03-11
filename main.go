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

	"libost/sticker_go/commands"
	"libost/sticker_go/config"
	C "libost/sticker_go/constants"
	"libost/sticker_go/database"
	"libost/sticker_go/stickers"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	_ "time/tzdata"
)

func main() {
	// 1. 创建 Bot 实例
	cfg, err := config.Init()
	if err != nil {
		log.Fatal(err)
	}
	if cfg == nil {
		log.Fatal("config initialization returned nil config")
	}
	token := cfg.Token
	if strings.TrimSpace(token) == "" || token == "YOUR_TOKEN_HERE" {
		log.Fatal("config.yaml token is empty or still using the placeholder value")
	}
	b, err := gotgbot.NewBot(token, &gotgbot.BotOpts{
		BotClient: &gotgbot.BaseBotClient{
			Client: http.Client{}, // 可以配置代理
		},
	})
	if err != nil {
		panic("failed to create bot: " + err.Error())
	}
	_, err = database.Init("init", 0, nil) // 初始化数据库连接
	if err != nil {
		panic("failed to initialize database: " + err.Error())
	}
	cacheDir := C.CacheDir
	cacheInfo, err := os.Stat(cacheDir) // 检查缓存目录是否存在
	if os.IsNotExist(err) || (err == nil && !cacheInfo.IsDir()) {
		err = os.Mkdir(cacheDir, 0755)
		if err != nil {
			panic("failed to create cache directory: " + err.Error())
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
		panic("ffmpeg is not installed or not in PATH: " + err.Error())
	}

	// 2. 创建分发器 (Dispatcher)
	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{
		Error: func(b *gotgbot.Bot, ctx *ext.Context, err error) ext.DispatcherAction {
			log.Println("发生错误:", err.Error())
			return ext.DispatcherActionNoop
		},
		MaxRoutines: ext.DefaultMaxRoutines,
	})

	// 3. 创建更新器 (Updater) 关联分发器
	updater := ext.NewUpdater(dispatcher, nil)

	// 4. 添加处理器 (Handler)
	commands.AddHandlers(dispatcher)
	stickers.AddHandlers(dispatcher)

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
		panic("failed to start polling: " + err.Error())
	}

	fmt.Printf("%s 已经启动...\n", b.User.Username)
	updater.Idle() // 阻塞直到进程被关闭
}

func cleanCache(cacheDir string, cfg *config.Config) {
	files, err := os.ReadDir(cacheDir)
	if err != nil {
		log.Println("读取缓存目录失败:", err)
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
				log.Printf("删除过期缓存失败 [%s]: %v\n", path, err)
			} else {
				log.Printf("已清理过期缓存: %s\n", file.Name())
			}
		}
		totalSize += info.Size()
	}
	if totalSize > int64(cfg.CacheSizeLimitMB)*1024*1024 {
		log.Printf("缓存大小超过限制: %d MB\n", totalSize/1024/1024)
		err := os.RemoveAll(cacheDir)
		if err != nil {
			log.Printf("清理缓存失败: %v\n", err)
		} else {
			log.Println("已清理所有缓存文件")
		}
		os.Mkdir(cacheDir, 0755) // 重新创建缓存目录
	}
}
