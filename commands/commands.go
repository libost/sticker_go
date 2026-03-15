package commands

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"runtime/metrics"
	"syscall"
	"time"

	"libost/sticker_go/config"
	C "libost/sticker_go/constants"
	"libost/sticker_go/database"
	"libost/sticker_go/log"
	"libost/sticker_go/utils"
	V "libost/sticker_go/version"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
)

// AddHandlers 注册所有的命令行处理器
func AddHandlers(dispatcher *ext.Dispatcher) {
	dispatcher.AddHandler(handlers.NewCommand("start", start))
	dispatcher.AddHandler(handlers.NewCommand("help", help))
	dispatcher.AddHandler(handlers.NewCommand("usage", usage))
	dispatcher.AddHandler(handlers.NewCommand("getstats", getstats))
	dispatcher.AddHandler(handlers.NewCommand("setadmin", setadmin))
	dispatcher.AddHandler(handlers.NewCommand("reset", resetUsage))
	dispatcher.AddHandler(handlers.NewCommand("clearcache", clearCache))
	dispatcher.AddHandler(handlers.NewCommand("setcommands", setcommands))
	dispatcher.AddHandler(handlers.NewCommand("about", about))
	dispatcher.AddHandler(handlers.NewCommand("clearlogs", clearLogs))
	dispatcher.AddHandler(handlers.NewCommand("restart", restart))
	dispatcher.AddHandler(handlers.NewCommand("shutdown", shutdown))
}

// start 处理器函数
func start(b *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveChat.Type != "private" {
		_, err := ctx.EffectiveMessage.Reply(b, "请在私聊中使用这个命令哦！", nil)
		return err
	}
	_, err := ctx.EffectiveMessage.Reply(b, "您好！向我发送贴纸，我可以将其转换为图片或视频格式并发送给您。\n项目地址： https://github.com/libost/sticker_go", nil)
	db, err := database.Init("init", ctx.EffectiveUser.Id, nil)
	if err != nil {
		return err
	}
	if !db["exists"].(bool) {
		_, err = database.Init("create", ctx.EffectiveUser.Id, nil)
		if err != nil {
			return err
		}
	}
	log.Log(fmt.Sprintf("User %d triggered /start", ctx.EffectiveUser.Id), C.LogLevelInfo)
	return err
}

// help 处理器函数
func help(b *gotgbot.Bot, ctx *ext.Context) error {
	displayText := "/start - 开始使用机器人\n" +
		"/help - 获取帮助信息\n" +
		"/usage - 查看使用情况\n" +
		"/about - 查看版本信息"
	if ctx.EffectiveChat.Type != "private" {
		displayText = "请在私聊中使用这些命令哦！\n\n" + displayText
	}

	_, err := ctx.EffectiveMessage.Reply(b, displayText, nil)
	db, err := database.Init("init", ctx.EffectiveUser.Id, nil)
	if err != nil {
		return err
	}
	if !db["exists"].(bool) {
		_, err = database.Init("create", ctx.EffectiveUser.Id, nil)
		if err != nil {
			return err
		}
	}
	log.Log(fmt.Sprintf("User %d triggered /help", ctx.EffectiveUser.Id), C.LogLevelInfo)
	return err
}

// usage 处理器函数
func usage(b *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveChat.Type != "private" {
		_, err := ctx.EffectiveMessage.Reply(b, "请在私聊中使用这个命令哦！", nil)
		return err
	}
	data, err := database.Init("usage", ctx.EffectiveUser.Id, nil)
	if err != nil {
		return err
	}
	if !data["exists"].(bool) {
		log.Log(fmt.Sprintf("User %d attempted to trigger /usage without a valid user record", ctx.EffectiveUser.Id), C.LogLevelWarn)
		_, err := ctx.EffectiveMessage.Reply(b, "你还没有使用记录。", nil)
		if err != nil {
			return err
		}
		_, err = database.Init("create", ctx.EffectiveUser.Id, nil)
		if err != nil {
			return err
		}
		return err
	}
	cf, err := config.Init()
	if err != nil {
		return err
	}
	if (int(data["last_cycle_starts_at"].(float64)) + 24*3600) < int(time.Now().Unix()) {
		_, err = database.Init("reset_usage", ctx.EffectiveUser.Id, nil)
		if err != nil {
			return err
		}
		data["usage"] = float64(0)
		data["last_cycle_starts_at"] = float64(time.Now().Unix() + 24*3600)
	}
	limit := cf.General.Limit
	remaining := max(limit-int(data["usage"].(float64)), 0)
	nextRefresh := time.Unix(int64(data["last_cycle_starts_at"].(float64))+24*3600, 0).Format("2006-01-02 15:04:05")
	info := fmt.Sprintf("使用信息:\n已使用: %d次\n剩余: %d次\n下次刷新: %s",
		int(data["usage"].(float64)), remaining, nextRefresh)
	_, err = ctx.EffectiveMessage.Reply(b, info, nil)
	log.Log(fmt.Sprintf("User %d triggered /usage", ctx.EffectiveUser.Id), C.LogLevelInfo)
	return err
}

// getstats 处理器函数
func getstats(b *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveChat.Type != "private" {
		return nil // 仅允许在私聊中使用 /getstats 命令，忽略群聊和频道中的命令
	}
	data, err := database.Init("user_group", ctx.EffectiveUser.Id, nil)
	if err != nil {
		return err
	}
	if !data["exists"].(bool) {
		log.Log(fmt.Sprintf("User %d attempted to trigger /getstats without a valid user record", ctx.EffectiveUser.Id), C.LogLevelWarn)
		_, err := ctx.EffectiveMessage.Reply(b, "你没有权限使用这个命令。", nil)
		database.Init("create", ctx.EffectiveUser.Id, nil)
		return err
	}
	if data["user_group"].(string) != "admin" {
		log.Log(fmt.Sprintf("User %d attempted to trigger /getstats without permission", ctx.EffectiveUser.Id), C.LogLevelWarn)
		_, err := ctx.EffectiveMessage.Reply(b, "你没有权限使用这个命令。", nil)
		return err
	}
	// 这里可以添加管理员才能看到的统计信息
	stats, err := database.Init("stats", ctx.EffectiveUser.Id, nil)
	if err != nil {
		return err
	}
	cacheDir := C.CacheDir
	cacheSize, err := utils.GetDirSize(cacheDir)
	if err != nil {
		cacheSize = 0
	}
	logDir := C.LogDir
	logSize, err := utils.GetDirSize(logDir)
	if err != nil {
		logSize = 0
	}
	samples := make([]metrics.Sample, 1)
	samples[0].Name = "/memory/classes/heap/objects:bytes"
	metrics.Read(samples)
	var memoryBytes uint64
	switch samples[0].Value.Kind() {
	case metrics.KindUint64:
		memoryBytes = samples[0].Value.Uint64()
	case metrics.KindFloat64:
		memoryBytes = uint64(samples[0].Value.Float64())
	default:
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		memoryBytes = mem.Alloc
	}
	info := fmt.Sprintf("管理员统计信息:\n总用户数: %d\n总使用次数: %d\n当前缓存占用: %d MB\n当前内存使用: %d MB\n当前日志占用: %d MB",
		int(stats["stats"].(map[string]any)["total_users"].(float64)),
		int(stats["stats"].(map[string]any)["total_usage"].(float64)),
		cacheSize/1024/1024,
		memoryBytes/1024/1024,
		logSize/1024/1024)
	_, err = ctx.EffectiveMessage.Reply(b, info, nil)
	log.Log(fmt.Sprintf("User %d triggered /getstats", ctx.EffectiveUser.Id), C.LogLevelInfo)
	return err

}

// setadmin 处理器函数
func setadmin(b *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveChat.Type != "private" {
		return nil // 仅允许在私聊中使用 /setadmin 命令，忽略群聊和频道中的命令
	}
	data, err := database.Init("user_group", ctx.EffectiveUser.Id, nil)
	if err != nil {
		return err
	}
	if !data["exists"].(bool) {
		log.Log(fmt.Sprintf("User %d attempted to trigger /setadmin without a valid user record", ctx.EffectiveUser.Id), C.LogLevelWarn)
		_, err := ctx.EffectiveMessage.Reply(b, "正在创建用户记录。\n请重试你刚才的命令。", nil)
		database.Init("create", ctx.EffectiveUser.Id, nil)
		return err
	}
	if data["user_group"].(string) == "admin" {
		_, err := ctx.EffectiveMessage.Reply(b, "你已经是管理员了。", nil)
		log.Log(fmt.Sprintf("User %d is already an admin", ctx.EffectiveUser.Id), C.LogLevelInfo)
		return err
	}
	arg := ctx.Args()
	if len(arg) == 1 {
		log.Log(fmt.Sprintf("User %d attempted to trigger /setadmin without providing a key", ctx.EffectiveUser.Id), C.LogLevelWarn)
		_, err := ctx.EffectiveMessage.Reply(b, "请提供密钥。", nil)
		return err
	}
	cf, err := config.Init()
	if err != nil {
		return err
	}
	if arg[1] != cf.General.Adminkey {
		log.Log(fmt.Sprintf("User %d attempted to trigger /setadmin with incorrect key", ctx.EffectiveUser.Id), C.LogLevelWarn)
		displayText := "密钥错误，你没有权限成为管理员。"
		_, err := ctx.EffectiveMessage.Reply(b, displayText, nil)
		return err
	}
	_, err = database.Init("set_group", ctx.EffectiveUser.Id, map[string]any{"group": "admin"})
	if err != nil {
		return err
	}
	_, err = ctx.EffectiveMessage.Reply(b, "你现在是管理员了！", nil)
	log.Log(fmt.Sprintf("User %d triggered /setadmin", ctx.EffectiveUser.Id), C.LogLevelInfo)
	return err
}

func resetUsage(b *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveChat.Type != "private" {
		return nil // 仅允许在私聊中使用 /reset 命令，忽略群聊和频道中的命令
	}
	data, err := database.Init("user_group", ctx.EffectiveUser.Id, nil)
	if err != nil {
		return err
	}
	if !data["exists"].(bool) {
		log.Log(fmt.Sprintf("User %d attempted to trigger /reset without a valid user record", ctx.EffectiveUser.Id), C.LogLevelWarn)
		_, err := ctx.EffectiveMessage.Reply(b, "你没有权限使用这个命令。", nil)
		database.Init("create", ctx.EffectiveUser.Id, nil)
		return err
	}
	if data["user_group"].(string) != "admin" {
		log.Log(fmt.Sprintf("User %d attempted to trigger /reset without permission", ctx.EffectiveUser.Id), C.LogLevelWarn)
		_, err := ctx.EffectiveMessage.Reply(b, "你没有权限使用这个命令。", nil)
		return err
	}
	database.Init("reset_usage", ctx.EffectiveUser.Id, nil)
	_, err = ctx.EffectiveMessage.Reply(b, "使用记录已重置！", nil)
	log.Log(fmt.Sprintf("User %d triggered /reset", ctx.EffectiveUser.Id), C.LogLevelInfo)
	return err
}

func clearCache(b *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveChat.Type != "private" {
		return nil // 仅允许在私聊中使用 /clearcache 命令，忽略群聊和频道中的命令
	}
	data, err := database.Init("user_group", ctx.EffectiveUser.Id, nil)
	if err != nil {
		return err
	}
	if !data["exists"].(bool) {
		log.Log(fmt.Sprintf("User %d attempted to trigger /clearcache without a valid user record", ctx.EffectiveUser.Id), C.LogLevelWarn)
		_, err := ctx.EffectiveMessage.Reply(b, "你没有权限使用这个命令。", nil)
		database.Init("create", ctx.EffectiveUser.Id, nil)
		return err
	}
	if data["user_group"].(string) != "admin" {
		log.Log(fmt.Sprintf("User %d attempted to trigger /clearcache without permission", ctx.EffectiveUser.Id), C.LogLevelWarn)
		_, err := ctx.EffectiveMessage.Reply(b, "你没有权限使用这个命令。", nil)
		return err
	}
	cacheDir := C.CacheDir
	err = os.RemoveAll(cacheDir)
	if err != nil {
		return err
	}
	err = os.Mkdir(cacheDir, 0755)
	if err != nil {
		return err
	}
	_, err = ctx.EffectiveMessage.Reply(b, "缓存已清空！", nil)
	log.Log(fmt.Sprintf("User %d triggered /clearcache", ctx.EffectiveUser.Id), C.LogLevelInfo)
	return err

}

func setcommands(b *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveChat.Type != "private" {
		return nil // 仅允许在私聊中使用 /setcommands 命令，忽略群聊和频道中的命令
	}
	data, err := database.Init("user_group", ctx.EffectiveUser.Id, nil)
	if err != nil {
		return err
	}
	if !data["exists"].(bool) {
		log.Log(fmt.Sprintf("User %d attempted to trigger /setcommands without a valid user record", ctx.EffectiveUser.Id), C.LogLevelWarn)
		_, err := ctx.EffectiveMessage.Reply(b, "你没有权限使用这个命令。", nil)
		database.Init("create", ctx.EffectiveUser.Id, nil)
		return err
	}
	if data["user_group"].(string) != "admin" {
		log.Log(fmt.Sprintf("User %d attempted to trigger /setcommands without permission", ctx.EffectiveUser.Id), C.LogLevelWarn)
		_, err := ctx.EffectiveMessage.Reply(b, "你没有权限使用这个命令。", nil)
		return err
	}
	commands := []gotgbot.BotCommand{
		{Command: "start", Description: "开始使用机器人"},
		{Command: "help", Description: "获取帮助信息"},
		{Command: "usage", Description: "查看使用情况"},
		{Command: "about", Description: "查看版本信息"},
	}
	_, err = b.SetMyCommands(commands, nil)
	if err != nil {
		return err
	}
	_, err = ctx.EffectiveMessage.Reply(b, "命令列表已更新！", nil)
	log.Log(fmt.Sprintf("User %d triggered /setcommands", ctx.EffectiveUser.Id), C.LogLevelInfo)
	return err
}

func about(b *gotgbot.Bot, ctx *ext.Context) error {
	displayText := fmt.Sprintf("版本: <code>%s</code>\n构建时间: <code>%s</code>\nGit 提交: <code>%s</code>\n分支: <code>%s</code>\n项目地址: https://github.com/libost/sticker_go", V.Version, V.BuildTime, V.GitCommit, V.Branch)
	if ctx.EffectiveChat.Type != "private" {
		displayText = "检测到群聊环境，大部分功能已禁用\n\n" + displayText
	}
	if ctx.EffectiveChat.Type == "channel" {
		return nil // 频道中不响应 /about 命令
	}
	_, err := ctx.EffectiveMessage.Reply(b, displayText, &gotgbot.SendMessageOpts{
		ParseMode: "HTML",
	})
	log.Log(fmt.Sprintf("User %d triggered /about", ctx.EffectiveUser.Id), C.LogLevelInfo)
	return err
}

func clearLogs(b *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveChat.Type != "private" {
		return nil // 仅允许在私聊中使用 /clearlogs 命令，忽略群聊和频道中的命令
	}
	data, err := database.Init("user_group", ctx.EffectiveUser.Id, nil)
	if err != nil {
		return err
	}
	if !data["exists"].(bool) {
		log.Log(fmt.Sprintf("User %d attempted to trigger /clearlogs without a valid user record", ctx.EffectiveUser.Id), C.LogLevelWarn)
		_, err := ctx.EffectiveMessage.Reply(b, "你没有权限使用这个命令。", nil)
		database.Init("create", ctx.EffectiveUser.Id, nil)
		return err
	}
	if data["user_group"].(string) != "admin" {
		log.Log(fmt.Sprintf("User %d attempted to trigger /clearlogs without permission", ctx.EffectiveUser.Id), C.LogLevelWarn)
		_, err := ctx.EffectiveMessage.Reply(b, "你没有权限使用这个命令。", nil)
		return err
	}
	inlineKeyboard := &gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
			{
				{Text: "确认", CallbackData: "clear_logs_confirm"},
				{Text: "取消", CallbackData: "clear_logs_cancel"},
			},
		},
	}
	_, err = ctx.EffectiveMessage.Reply(b, "真的要清除日志吗？\n此操作不可逆！", &gotgbot.SendMessageOpts{
		ReplyMarkup: inlineKeyboard,
	})
	log.Log(fmt.Sprintf("User %d triggered /clearlogs", ctx.EffectiveUser.Id), C.LogLevelInfo)
	return err
}

func restart(b *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveChat.Type != "private" {
		return nil // 仅允许在私聊中使用 /restart 命令，忽略群聊和频道中的命令
	}
	data, err := database.Init("user_group", ctx.EffectiveUser.Id, nil)
	if err != nil {
		return err
	}
	if !data["exists"].(bool) {
		log.Log(fmt.Sprintf("User %d attempted to trigger /restart without a valid user record", ctx.EffectiveUser.Id), C.LogLevelWarn)
		_, err := ctx.EffectiveMessage.Reply(b, "你没有权限使用这个命令。", nil)
		database.Init("create", ctx.EffectiveUser.Id, nil)
		return err
	}
	if data["user_group"].(string) != "admin" {
		log.Log(fmt.Sprintf("User %d attempted to trigger /restart without permission", ctx.EffectiveUser.Id), C.LogLevelWarn)
		_, err := ctx.EffectiveMessage.Reply(b, "你没有权限使用这个命令。", nil)
		return err
	}
	_, err = ctx.EffectiveMessage.Reply(b, "正在重启机器人...", nil)
	log.Log(fmt.Sprintf("User %d triggered /restart", ctx.EffectiveUser.Id), C.LogLevelWarn)
	if err != nil {
		return err
	}
	ops := runtime.GOOS
	switch ops {
	case "windows":
		pathExe, err := os.Executable()
		if err != nil {
			return err
		}
		cmd := exec.Command(pathExe, os.Args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		cmd.Env = os.Environ()
		err = cmd.Start()
		if err != nil {
			return err
		}
		os.Exit(0)
	case "linux", "darwin":
		_, exists := os.LookupEnv("INVOCATION_ID")
		if exists {
			os.Exit(0)
		} else {
			pathExe, _ := os.Executable()
			args := os.Args
			env := os.Environ()
			err = syscall.Exec(pathExe, args, env)
		}
	}
	return nil
}

func shutdown(b *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveChat.Type != "private" {
		return nil // 仅允许在私聊中使用 /shutdown 命令，忽略群聊和频道中的命令
	}
	data, err := database.Init("user_group", ctx.EffectiveUser.Id, nil)
	if err != nil {
		return err
	}
	if !data["exists"].(bool) {
		log.Log(fmt.Sprintf("User %d attempted to trigger /shutdown without a valid user record", ctx.EffectiveUser.Id), C.LogLevelWarn)
		_, err := ctx.EffectiveMessage.Reply(b, "你没有权限使用这个命令。", nil)
		database.Init("create", ctx.EffectiveUser.Id, nil)
		return err
	}
	if data["user_group"].(string) != "admin" {
		log.Log(fmt.Sprintf("User %d attempted to trigger /shutdown without permission", ctx.EffectiveUser.Id), C.LogLevelWarn)
		_, err := ctx.EffectiveMessage.Reply(b, "你没有权限使用这个命令。", nil)
		return err
	}
	displayText := "真的要关闭机器人吗？\n此操作不可逆！"
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		_, exists := os.LookupEnv("INVOCATION_ID")
		if exists {
			displayText += "\n警告：当前环境似乎是 systemd 管理的服务，确认后机器人将会自动重启。"
		}
	}
	inlineKeyboard := &gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
			{
				{Text: "确认", CallbackData: "shutdown_confirm"},
				{Text: "取消", CallbackData: "shutdown_cancel"},
			},
		},
	}
	_, err = ctx.EffectiveMessage.Reply(b, displayText, &gotgbot.SendMessageOpts{
		ReplyMarkup: inlineKeyboard,
	})
	log.Log(fmt.Sprintf("User %d triggered /shutdown", ctx.EffectiveUser.Id), C.LogLevelWarn)
	if err != nil {
		return err
	}
	return nil
}
