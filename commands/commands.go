package commands

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"runtime/metrics"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/libost/sticker_go/config"
	C "github.com/libost/sticker_go/constants"
	"github.com/libost/sticker_go/database"
	"github.com/libost/sticker_go/log"
	S "github.com/libost/sticker_go/stickers"
	"github.com/libost/sticker_go/utils"
	V "github.com/libost/sticker_go/version"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/google/uuid"
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
	dispatcher.AddHandler(handlers.NewCommand("admin", adminCommands))
	dispatcher.AddHandler(handlers.NewCommand("get", getCommand))
	dispatcher.AddHandler(handlers.NewCommand("donate", donate))
	dispatcher.AddHandler(handlers.NewCommand("donaterecord", getAllDonates))
	dispatcher.AddHandler(handlers.NewCommand("refund", refund))
}

func checkAdmin(b *gotgbot.Bot, ctx *ext.Context, command string) (bool, error) {
	data, err := database.Init("user_group", ctx.EffectiveUser.Id, nil)
	if err != nil {
		return false, err
	}
	if !data["exists"].(bool) {
		log.Log(fmt.Sprintf("User %d attempted to trigger /%s without a valid user record", ctx.EffectiveUser.Id, command), C.LogLevelWarn)
		_, err := ctx.EffectiveMessage.Reply(b, "你没有权限使用这个命令。", nil)
		database.Init("create", ctx.EffectiveUser.Id, nil)
		return false, err
	}
	if data["user_group"].(string) != "admin" {
		log.Log(fmt.Sprintf("User %d attempted to trigger /%s without permission", ctx.EffectiveUser.Id, command), C.LogLevelWarn)
		_, err := ctx.EffectiveMessage.Reply(b, "你没有权限使用这个命令。", nil)
		return false, err
	}
	return true, nil
}

func isAdminUser(userID int64) (bool, error) {
	data, err := database.Init("user_group", userID, nil)
	if err != nil {
		return false, err
	}
	if !data["exists"].(bool) {
		_, err = database.Init("create", userID, nil)
		if err != nil {
			return false, err
		}
		return false, nil
	}
	return data["user_group"].(string) == "admin", nil
}

func anyToInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		return int64(n), true
	default:
		return 0, false
	}
}

// start 处理器函数
func start(b *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveChat.Type != "private" {
		_, err := ctx.EffectiveMessage.Reply(b, "您好！使用命令 /get 并回复一条贴纸信息，我可以将其转换为图片或视频格式并发送给您。", nil)
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
	cf, err := config.Init()
	if err != nil {
		return err
	}
	if cf.Donation.Enabled {
		displayText += "\n/donate - 向我们捐赠"
	}

	_, err = ctx.EffectiveMessage.Reply(b, displayText, nil)
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
	if ctx.EffectiveChat.Type != "private" && ctx.EffectiveChat.Type != "group" && ctx.EffectiveChat.Type != "supergroup" {
		_, err := ctx.EffectiveMessage.Reply(b, "请在私聊/群组中使用这个命令哦！", nil)
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
	isAdmin, err := checkAdmin(b, ctx, "getstats")
	if isAdmin != true {
		return nil
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
	info := fmt.Sprintf("管理员统计信息:\n总用户数: %d\n总使用次数: %d\n近一周使用次数: %d\n当前缓存占用: %d MB\n当前内存使用: %d MB\n当前日志占用: %d MB",
		int(stats["stats"].(map[string]any)["total_users"].(float64)),
		int(stats["stats"].(map[string]any)["total_usage"].(float64)),
		int(stats["stats"].(map[string]any)["weekly_usage"].(float64)),
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
	isAdmin, err := checkAdmin(b, ctx, "reset")
	if isAdmin != true {
		return nil
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
	isAdmin, err := checkAdmin(b, ctx, "clearcache")
	if isAdmin != true {
		return nil
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
	isAdmin, err := checkAdmin(b, ctx, "setcommands")
	if isAdmin != true {
		return nil
	}
	commands := []gotgbot.BotCommand{
		{Command: "start", Description: "开始使用机器人"},
		{Command: "help", Description: "获取帮助信息"},
		{Command: "usage", Description: "查看使用情况"},
		{Command: "about", Description: "查看版本信息"},
	}
	cf, err := config.Init()
	if cf.Donation.Enabled {
		commands = append(commands, gotgbot.BotCommand{Command: "donate", Description: "向我们捐赠"})
	}
	opts := gotgbot.SetMyCommandsOpts{
		Scope: gotgbot.BotCommandScopeAllPrivateChats{},
	}
	_, err = b.SetMyCommands(commands, &opts)
	if err != nil {
		return err
	}
	commands = []gotgbot.BotCommand{
		{Command: "start", Description: "开始使用贴纸下载机器人"},
		{Command: "get", Description: "获取回复信息中的贴纸"},
		{Command: "help", Description: "获取贴纸下载机器人的帮助信息"},
		{Command: "usage", Description: "查看使用情况"},
		{Command: "about", Description: "查看版本信息"},
	}
	opts = gotgbot.SetMyCommandsOpts{
		Scope: gotgbot.BotCommandScopeAllGroupChats{},
	}
	_, err = b.SetMyCommands(commands, &opts)
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
		displayText = "检测到群聊环境，部分功能已禁用\n\n" + displayText
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
	isAdmin, err := checkAdmin(b, ctx, "clearlogs")
	if isAdmin != true {
		return nil
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
	isAdmin, err := checkAdmin(b, ctx, "restart")
	if isAdmin != true {
		return nil
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
	isAdmin, err := checkAdmin(b, ctx, "shutdown")
	if isAdmin != true {
		return nil
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

func adminCommands(b *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveChat.Type != "private" {
		return nil // 仅允许在私聊中使用 /admin 命令，忽略群聊和频道中的命令
	}
	isAdmin, err := checkAdmin(b, ctx, "admin")
	if isAdmin != true {
		return nil
	}
	displayText := "管理员命令列表：\n" +
		"/getstats - 获取统计信息\n" +
		"/reset - 重置当前用户的使用记录\n" +
		"/clearcache - 清空缓存文件\n" +
		"/setcommands - 设置机器人命令列表\n" +
		"/clearlogs - 清除日志文件\n" +
		"/donaterecord - 查看所有捐赠记录\n" +
		"/restart - 重启机器人\n" +
		"/shutdown - 关闭机器人"
	_, err = ctx.EffectiveMessage.Reply(b, displayText, nil)
	log.Log(fmt.Sprintf("User %d triggered /admin", ctx.EffectiveUser.Id), C.LogLevelInfo)
	return err
}

func getCommand(b *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveChat.Type == "private" {
		_, err := ctx.EffectiveMessage.Reply(b, "请在群聊中使用这个命令哦！", nil)
		return err
	}
	if ctx.EffectiveChat.Type != "group" && ctx.EffectiveChat.Type != "supergroup" {
		return nil // 仅允许在群聊中使用 /get 命令，忽略私聊和频道中的命令
	}
	if ctx.EffectiveMessage.ReplyToMessage == nil || ctx.EffectiveMessage.ReplyToMessage.Sticker == nil {
		_, err := ctx.EffectiveMessage.Reply(b, "请回复一个贴纸消息并使用这个命令哦！", nil)
		return err
	}
	cf, err := config.Init()
	if err != nil {
		return err
	}
	currentUsage, err := database.Init("usage", ctx.EffectiveUser.Id, nil)
	if err != nil {
		return err
	}
	if !currentUsage["exists"].(bool) {
		database.Init("create", ctx.EffectiveUser.Id, nil)
		currentUsage["usage"] = float64(0)
	}
	limit := cf.General.Limit
	if int(currentUsage["usage"].(float64)) >= limit {
		_, err := ctx.EffectiveMessage.Reply(b, fmt.Sprintf("您已达到使用上限 (%d 次)，请稍后再试！", limit), nil)
		return err
	}
	if cf.Subscription.Enabled {
		err := utils.SubscribeCheck(b, ctx.EffectiveUser.Id)
		if err != nil {
			channel := strings.TrimPrefix(cf.Subscription.Channel, "@")
			if errors.Is(err, utils.ErrUserNotSubscribed) {
				displayText := fmt.Sprintf("🤖 为了支持我们的项目并继续提供免费服务，请先加入<a href=\"https://t.me/%s\">官方频道</a> %s 后再使用本功能哦！\n✅ 加入后请再次点击您刚才发送的命令即可继续。", channel, cf.Subscription.Channel)
				_, replyErr := ctx.EffectiveMessage.Reply(b, displayText, &gotgbot.SendMessageOpts{
					ParseMode: "HTML",
				})
				if replyErr != nil {
					return replyErr
				}
				return nil
			}
			displayText := fmt.Sprintf("🤖 订阅检查失败，请稍后重试。\n您确定订阅了我们的<a href=\"https://t.me/%s\">官方频道</a> %s 吗？", channel, cf.Subscription.Channel)
			_, replyErr := ctx.EffectiveMessage.Reply(b, displayText, &gotgbot.SendMessageOpts{
				ParseMode: "HTML",
			})
			if replyErr != nil {
				return replyErr
			}
		}
	}
	msg, err := ctx.EffectiveMessage.Reply(b, "正在发送贴纸文件，请稍候...", nil)
	if err != nil {
		return err
	}
	sticker := ctx.EffectiveMessage.ReplyToMessage.Sticker
	filePath, _, err := S.GetSticker(b, sticker, ctx.EffectiveUser.Id, cf)
	if err != nil {
		return err
	}
	fileSend, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer fileSend.Close()
	replyparameters := &gotgbot.SendDocumentOpts{
		ReplyParameters: &gotgbot.ReplyParameters{
			MessageId: ctx.EffectiveMessage.MessageId,
		},
	}
	_, err = b.SendDocument(ctx.EffectiveChat.Id, gotgbot.InputFileByReader(fileSend.Name(), fileSend), replyparameters)
	if err != nil {
		return err
	}
	_, _, err = b.EditMessageText("贴纸文件发送完成！\n想获得整套贴纸包？请在私聊中与机器人对话。", &gotgbot.EditMessageTextOpts{
		ChatId:    msg.Chat.Id,
		MessageId: msg.MessageId,
	})
	database.Init("usageRecord", ctx.EffectiveUser.Id, map[string]any{"usage": 1})
	return err
}

func donate(b *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveChat.Type != "private" {
		return nil // 仅允许在私聊中使用 /donate 命令，忽略群聊和频道中的命令
	}
	isAdmin, err := isAdminUser(ctx.EffectiveUser.Id)
	if err != nil {
		return err
	}
	if isAdmin {
		_, err := ctx.EffectiveMessage.Reply(b, "管理员不需要捐赠哦！", nil)
		return err
	}
	cf, err := config.Init()
	if err != nil {
		return err
	}
	if !cf.Donation.Enabled {
		_, err := ctx.EffectiveMessage.Reply(b, "感谢您的好意，但目前我们还不接受捐赠哦！", nil)
		if err != nil {
			return err
		}
		return nil
	}
	arg := ctx.Args()
	price := gotgbot.LabeledPrice{
		Label:  "支持开发者",
		Amount: 50, // 金额单位为Telegram Stars
	}
	if len(arg) > 1 {
		amount, err := strconv.ParseInt(arg[1], 10, 64)
		if err == nil {
			price.Amount = amount
		}
		if amount < int64(cf.Donation.AmountRestrict.Min) {
			price.Amount = int64(cf.Donation.AmountRestrict.Min)
		}
		if amount > int64(cf.Donation.AmountRestrict.Max) {
			price.Amount = int64(cf.Donation.AmountRestrict.Max)
		}
	}
	payloadUuid := uuid.New().String()
	payload := fmt.Sprintf("donate_%s", payloadUuid)
	_, err = b.SendInvoice(ctx.EffectiveUser.Id, cf.Donation.Title, cf.Donation.Description, payload, "XTR", []gotgbot.LabeledPrice{price}, &gotgbot.SendInvoiceOpts{
		ProtectContent: true,
	})
	if err != nil {
		return err
	}
	database.Init("donateInit", ctx.EffectiveUser.Id, map[string]any{"amount": price.Amount, "payload": payload})
	return nil
}

func getAllDonates(b *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveChat.Type != "private" {
		return nil // 仅允许在私聊中使用这个命令，忽略群聊和频道中的命令
	}
	isAdmin, err := checkAdmin(b, ctx, "getalldonates")
	if isAdmin != true {
		return nil
	}
	donates, err := database.Init("get_all_donates", ctx.EffectiveUser.Id, nil)
	if err != nil {
		return err
	}
	if len(donates["donates"].([]map[string]any)) == 0 {
		_, err := ctx.EffectiveMessage.Reply(b, "目前没有任何捐赠记录。", nil)
		return err
	}
	args := ctx.Args()
	var displayText strings.Builder
	displayText.WriteString("所有捐赠记录：\n")
	recordLength := 0
	for _, donate := range donates["donates"].([]map[string]any) {
		timestamp, ok := anyToInt64(donate["timestamp"])
		if !ok {
			continue
		}
		if len(args) > 1 {
			if args[1] != donates["status"] {
				continue
			}
		}
		userID, _ := anyToInt64(donate["user_id"])
		amount, _ := anyToInt64(donate["amount"])
		timeFormatted := time.Unix(timestamp, 0).Format("2006-01-02 15:04:05")
		fmt.Fprintf(&displayText, "用户ID: %d, 金额: %d, 时间: %s，状态: %s\n", userID, amount, timeFormatted, donate["status"])
		recordLength++
	}
	if recordLength == 0 {
		_, err := ctx.EffectiveMessage.Reply(b, "没有符合条件的捐赠记录。", nil)
		return err
	}
	_, err = ctx.EffectiveMessage.Reply(b, displayText.String(), nil)
	return err

}

func refund(b *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveChat.Type != "private" {
		return nil // 仅允许在私聊中使用 /refund 命令，忽略群聊和频道中的命令
	}
	isAdmin, err := isAdminUser(ctx.EffectiveUser.Id)
	if err != nil {
		return err
	}
	if isAdmin {
		_, err := ctx.EffectiveMessage.Reply(b, "管理员无需退款", nil)
		if err != nil {
			return err
		}
		return nil
	}
	data, err := database.Init("getUserDonations", ctx.EffectiveUser.Id, nil)
	if err != nil {
		return err
	}
	if len(data["donations"].([]map[string]any)) == 0 {
		_, err := ctx.EffectiveMessage.Reply(b, "您没有任何捐赠记录。", nil)
		return err
	}
	timeNow := time.Now().Unix()
	refundableDonations := make([]map[string]any, 0)
	for _, donation := range data["donations"].([]map[string]any) {
		timestamp, ok := anyToInt64(donation["timestamp"])
		if !ok {
			continue
		}
		if timeNow-timestamp <= int64(C.RefundPeriod)*24*3600 {
			refundableDonations = append(refundableDonations, donation)
		}
	}
	if len(refundableDonations) == 0 {
		_, err := ctx.EffectiveMessage.Reply(b, "很抱歉，您的捐赠已超过可退款期限。", nil)
		return err
	}
	var displayText strings.Builder
	InlineKeyboard := [][]gotgbot.InlineKeyboardButton{}
	displayText.WriteString("您有以下捐赠记录符合退款条件：\n")
	for i, donation := range refundableDonations {
		timestamp, ok := anyToInt64(donation["timestamp"])
		if !ok {
			continue
		}
		telegramChargeID, ok := donation["telegram_payment_charge_id"].(string)
		if !ok || telegramChargeID == "" {
			continue
		}
		timeFormatted := time.Unix(timestamp, 0).Format("2006-01-02 15:04:05")
		InlineKeyboard = append(InlineKeyboard, []gotgbot.InlineKeyboardButton{
			{
				Text:         fmt.Sprintf("申请退款 %d", i+1),
				CallbackData: fmt.Sprintf("refund_apply_%s", telegramChargeID),
			},
		})

		amount, _ := anyToInt64(donation["amount"])
		fmt.Fprintf(&displayText, "%d. 金额: %d, 时间: %s\n", i+1, amount, timeFormatted)
	}
	if len(InlineKeyboard) == 0 {
		_, err := ctx.EffectiveMessage.Reply(b, "没有可用于申请退款的有效捐赠记录。", nil)
		return err
	}

	_, err = ctx.EffectiveMessage.Reply(b, displayText.String(), &gotgbot.SendMessageOpts{
		ReplyMarkup: &gotgbot.InlineKeyboardMarkup{
			InlineKeyboard: InlineKeyboard,
		},
	})
	return err
}
