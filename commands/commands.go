package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"runtime/metrics"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/libost/sticker_go/callback"
	"github.com/libost/sticker_go/config"
	C "github.com/libost/sticker_go/constants"
	"github.com/libost/sticker_go/database"
	I "github.com/libost/sticker_go/i18n"
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
	dispatcher.AddHandler(handlers.NewCommand("upgrade", upgrade))
	dispatcher.AddHandler(handlers.NewCommand("lang", languages))
}

func checkAdmin(b *gotgbot.Bot, ctx *ext.Context, command string) (bool, error) {
	data, err := database.Init("user_group", ctx.EffectiveUser.Id, nil)
	if err != nil {
		return false, err
	}
	langCode := I.LangCodePrefer(ctx.EffectiveUser.Id, ctx.EffectiveUser.LanguageCode)
	switch command {
	case "donate", "refund":
		if !data["exists"].(bool) {
			log.Log(fmt.Sprintf("User %d attempted to trigger /%s without a valid user record", ctx.EffectiveUser.Id, command), C.LogLevelWarn)
			database.Init("create", ctx.EffectiveUser.Id, nil)
			return false, nil
		} else if data["user_group"].(string) != "admin" {
			return false, nil
		} else {
			return true, nil
		}
	default:
		if !data["exists"].(bool) {
			log.Log(fmt.Sprintf("User %d attempted to trigger /%s without a valid user record", ctx.EffectiveUser.Id, command), C.LogLevelWarn)
			_, err := ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.permission_denied", langCode), nil)
			database.Init("create", ctx.EffectiveUser.Id, nil)
			return false, err
		}
		if data["user_group"].(string) != "admin" {
			log.Log(fmt.Sprintf("User %d attempted to trigger /%s without permission", ctx.EffectiveUser.Id, command), C.LogLevelWarn)
			_, err := ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.permission_denied", langCode), nil)
			return false, err
		}
		return true, nil
	}
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

func isTelegramLanguageCode(code string) bool {
	if len(code) != 2 {
		return false
	}
	for _, ch := range code {
		if ch < 'a' || ch > 'z' {
			return false
		}
	}
	return true
}

func normalizeTelegramLanguageCode(primaryCode, altCode string) (string, bool) {
	primaryCode = strings.ToLower(strings.TrimSpace(primaryCode))
	altCode = strings.ToLower(strings.TrimSpace(altCode))

	if isTelegramLanguageCode(altCode) {
		return altCode, true
	}
	if isTelegramLanguageCode(primaryCode) {
		return primaryCode, true
	}
	return "", false
}

// start 处理器函数
func start(b *gotgbot.Bot, ctx *ext.Context) error {
	langCode := I.LangCodePrefer(ctx.EffectiveUser.Id, ctx.EffectiveUser.LanguageCode)
	args := ctx.Args()
	if len(args) > 1 {
		// 处理 start 参数，例如 deep linking
		cfg, err := config.Init()
		if err != nil {
			return err
		}
		if cfg.Subscription.Enabled {
			err := utils.SubscribeCheck(b, ctx.EffectiveUser.Id)
			if err != nil {
				channel := strings.TrimPrefix(cfg.Subscription.Channel, "@")
				if errors.Is(err, utils.ErrUserNotSubscribed) {
					displayText := fmt.Sprintf(I.GetLocalisedString("general.subscription_required", langCode), channel, cfg.Subscription.Channel)
					_, replyErr := ctx.EffectiveMessage.Reply(b, displayText, &gotgbot.SendMessageOpts{
						ParseMode: "HTML",
					})
					if replyErr != nil {
						return replyErr
					}
					return nil
				}
				displayText := fmt.Sprintf(I.GetLocalisedString("general.subscription_check_failed", langCode), channel, cfg.Subscription.Channel)
				_, replyErr := ctx.EffectiveMessage.Reply(b, displayText, &gotgbot.SendMessageOpts{
					ParseMode: "HTML",
				})
				if replyErr != nil {
					return replyErr
				}
				return nil
			}
		}
		param := args[1]
		log.Log(fmt.Sprintf("User %d triggered /start with parameter: %s", ctx.EffectiveUser.Id, param), C.LogLevelInfo)
		msg, err := ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.get_desc_processing", langCode), nil)
		if err != nil {
			return err
		}
		err = callback.GetPack(b, ctx, param, langCode, msg.GetMessageId())
		if err != nil {
			log.Log(fmt.Sprintf("User %d failed to get sticker pack with parameter %s: %v", ctx.EffectiveUser.Id, param, err), C.LogLevelError)
			_, err = ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.get_desc_failed", langCode), nil)
			return err
		}
		return nil
	}
	if ctx.EffectiveChat.Type != "private" {
		_, err := ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.start_desc_group", langCode), nil)
		return err
	}
	_, err := ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.start_desc_private", langCode), nil)
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
	langCode := I.LangCodePrefer(ctx.EffectiveUser.Id, ctx.EffectiveUser.LanguageCode)
	displayText := I.GetLocalisedString("commands.help_desc_private", langCode)
	cf, err := config.Init()
	if err != nil {
		return err
	}
	if cf.Donation.Enabled {
		displayText += "\n" + I.GetLocalisedString("commands.help_desc_donate_enabled", langCode)
	}
	if ctx.EffectiveChat.Type != "private" {
		displayText = I.GetLocalisedString("commands.help_desc_group", langCode)
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
	langCode := I.LangCodePrefer(ctx.EffectiveUser.Id, ctx.EffectiveUser.LanguageCode)
	if ctx.EffectiveChat.Type != "private" && ctx.EffectiveChat.Type != "group" && ctx.EffectiveChat.Type != "supergroup" {
		_, err := ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.groupchat_only", langCode), nil)
		return err
	}
	data, err := database.Init("usage", ctx.EffectiveUser.Id, nil)
	if err != nil {
		return err
	}
	if !data["exists"].(bool) {
		log.Log(fmt.Sprintf("User %d attempted to trigger /usage without a valid user record", ctx.EffectiveUser.Id), C.LogLevelWarn)
		_, err := ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.usage_desc_no_record", langCode), nil)
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
	userGroup, err := database.Init("user_group", ctx.EffectiveUser.Id, nil)
	if err != nil {
		return err
	}
	limit := cf.General.Limit
	var donationBonusExplanation string
	if userGroup["user_group"].(string) == "sponsor" && cf.Donation.BonusEnabled {
		limit = int(float64(limit) * C.DonationBonusMultiplier)
		donationBonusExplanation = fmt.Sprintf(I.GetLocalisedString("commands.usage_desc_multiply", langCode), C.DonationBonusMultiplier)
	}
	remaining := max(limit-int(data["usage"].(float64)), 0)
	nextRefresh := time.Unix(int64(data["last_cycle_starts_at"].(float64))+24*3600, 0).Format("2006-01-02 15:04:05")
	info := fmt.Sprintf(I.GetLocalisedString("commands.usage_desc", langCode),
		int(data["usage"].(float64)), remaining, nextRefresh, donationBonusExplanation)
	_, err = ctx.EffectiveMessage.Reply(b, info, nil)
	log.Log(fmt.Sprintf("User %d triggered /usage", ctx.EffectiveUser.Id), C.LogLevelInfo)
	return err
}

// getstats 处理器函数
func getstats(b *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveChat.Type != "private" {
		return nil // 仅允许在私聊中使用 /getstats 命令，忽略群聊和频道中的命令
	}
	langCode := I.LangCodePrefer(ctx.EffectiveUser.Id, ctx.EffectiveUser.LanguageCode)
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
	info := fmt.Sprintf(I.GetLocalisedString("commands.getstats_details", langCode),
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
	langCode := I.LangCodePrefer(ctx.EffectiveUser.Id, ctx.EffectiveUser.LanguageCode)
	if !data["exists"].(bool) {
		log.Log(fmt.Sprintf("User %d attempted to trigger /setadmin without a valid user record", ctx.EffectiveUser.Id), C.LogLevelWarn)
		_, err := ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.setadmin_no_records", langCode), nil)
		database.Init("create", ctx.EffectiveUser.Id, nil)
		return err
	}
	if data["user_group"].(string) == "admin" {
		_, err := ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.setadmin_already", langCode), nil)
		log.Log(fmt.Sprintf("User %d is already an admin", ctx.EffectiveUser.Id), C.LogLevelInfo)
		return err
	}
	arg := ctx.Args()
	if len(arg) == 1 {
		log.Log(fmt.Sprintf("User %d attempted to trigger /setadmin without providing a key", ctx.EffectiveUser.Id), C.LogLevelWarn)
		_, err := ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.setadmin_no_key", langCode), nil)
		return err
	}
	cf, err := config.Init()
	if err != nil {
		return err
	}
	if arg[1] != cf.General.Adminkey {
		log.Log(fmt.Sprintf("User %d attempted to trigger /setadmin with incorrect key", ctx.EffectiveUser.Id), C.LogLevelWarn)
		_, err := ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.setadmin_invalid_key", langCode), nil)
		return err
	}
	_, err = database.Init("set_group", ctx.EffectiveUser.Id, map[string]any{"group": "admin"})
	if err != nil {
		return err
	}
	_, err = ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.setadmin_success", langCode), nil)
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
	langCode := I.LangCodePrefer(ctx.EffectiveUser.Id, ctx.EffectiveUser.LanguageCode)
	database.Init("reset_usage", ctx.EffectiveUser.Id, nil)
	_, err = ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.reset_success", langCode), nil)
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
	err = os.RemoveAll(C.CacheDir)
	if err != nil {
		return err
	}
	err = os.Mkdir(C.CacheDir, 0755)
	if err != nil {
		return err
	}
	langCode := I.LangCodePrefer(ctx.EffectiveUser.Id, ctx.EffectiveUser.LanguageCode)
	_, err = ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.clearcache_success", langCode), nil)
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
	// TODO: language-specific commands
	supportedLanguages, _, err := I.GetAllSupportedLanguages()
	if err != nil {
		return err
	}
	for idx, lang := range supportedLanguages {
		keyIndex := idx + 1
		name := lang[fmt.Sprintf("name_%d", keyIndex)]
		code := lang[fmt.Sprintf("code_%d", keyIndex)]
		codeAlt := lang[fmt.Sprintf("code_alt_%d", keyIndex)]
		if name == "" || code == "" {
			continue
		}
		languageCode, ok := normalizeTelegramLanguageCode(code, codeAlt)
		if !ok {
			log.Log(fmt.Sprintf("Skipping private commands for language %s due to invalid Telegram language code (code=%s, code_alt=%s)", name, code, codeAlt), C.LogLevelWarn)
			continue
		}
		commands := []gotgbot.BotCommand{
			{Command: "start", Description: I.GetLocalisedString("commands.setcommands_desc_list[0]", code)},
			{Command: "help", Description: I.GetLocalisedString("commands.setcommands_desc_list[1]", code)},
			{Command: "usage", Description: I.GetLocalisedString("commands.setcommands_desc_list[2]", code)},
			{Command: "lang", Description: I.GetLocalisedString("commands.setcommands_desc_list[9]", code)},
			{Command: "about", Description: I.GetLocalisedString("commands.setcommands_desc_list[3]", code)},
		}
		cf, err := config.Init()
		if cf.Donation.Enabled {
			commands = append(commands, gotgbot.BotCommand{Command: "donate", Description: I.GetLocalisedString("commands.setcommands_desc_list[4]", code)})
		}
		if languageCode == "en" {
			languageCode = "" // Default language commands should be set with empty language code in Telegram
		}
		opts := gotgbot.SetMyCommandsOpts{
			Scope:        gotgbot.BotCommandScopeAllPrivateChats{},
			LanguageCode: languageCode,
		}
		_, err = b.SetMyCommands(commands, &opts)
		if err != nil {
			return err
		}
	}

	for idx, lang := range supportedLanguages {
		keyIndex := idx + 1
		name := lang[fmt.Sprintf("name_%d", keyIndex)]
		code := lang[fmt.Sprintf("code_%d", keyIndex)]
		code_alt := lang[fmt.Sprintf("code_alt_%d", keyIndex)]
		if name == "" || code == "" {
			continue
		}
		languageCode, ok := normalizeTelegramLanguageCode(code, code_alt)
		if !ok {
			log.Log(fmt.Sprintf("Skipping group commands for language %s due to invalid Telegram language code (code=%s, code_alt=%s)", name, code, code_alt), C.LogLevelWarn)
			continue
		}
		commands := []gotgbot.BotCommand{
			{Command: "start", Description: I.GetLocalisedString("commands.setcommands_desc_list[5]", code)},
			{Command: "get", Description: I.GetLocalisedString("commands.setcommands_desc_list[6]", code)},
			{Command: "help", Description: I.GetLocalisedString("commands.setcommands_desc_list[7]", code)},
			{Command: "usage", Description: I.GetLocalisedString("commands.setcommands_desc_list[2]", code)},
			{Command: "lang", Description: I.GetLocalisedString("commands.setcommands_desc_list[9]", code)},
			{Command: "about", Description: I.GetLocalisedString("commands.setcommands_desc_list[8]", code)},
		}
		if languageCode == "en" {
			languageCode = "" // Default language commands should be set with empty language code in Telegram
		}
		opts := gotgbot.SetMyCommandsOpts{
			Scope:        gotgbot.BotCommandScopeAllGroupChats{},
			LanguageCode: languageCode,
		}
		_, err = b.SetMyCommands(commands, &opts)
		if err != nil {
			return err
		}
	}
	langCode := I.LangCodePrefer(ctx.EffectiveUser.Id, ctx.EffectiveUser.LanguageCode)
	_, err = ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.setcommands_success", langCode), nil)
	log.Log(fmt.Sprintf("User %d triggered /setcommands", ctx.EffectiveUser.Id), C.LogLevelInfo)
	return err
}

func about(b *gotgbot.Bot, ctx *ext.Context) error {
	langCode := I.LangCodePrefer(ctx.EffectiveUser.Id, ctx.EffectiveUser.LanguageCode)
	displayText := fmt.Sprintf(I.GetLocalisedString("commands.about_desc", langCode), V.Version, V.BuildTime, V.GitCommit, V.Branch)
	if os.Getenv("IN_DOCKER") == "true" {
		displayText = I.GetLocalisedString("commands.about_desc_docker", langCode) + displayText
	}
	if ctx.EffectiveChat.Type != "private" {
		displayText = I.GetLocalisedString("commands.about_desc_group", langCode) + displayText
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
	langCode := I.LangCodePrefer(ctx.EffectiveUser.Id, ctx.EffectiveUser.LanguageCode)
	inlineKeyboard := &gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
			{
				{Text: I.GetLocalisedString("general.confirm", langCode), CallbackData: "clear_logs_confirm", Style: "danger"},
				{Text: I.GetLocalisedString("general.cancel", langCode), CallbackData: "clear_logs_cancel", Style: "primary"},
			},
		},
	}
	_, err = ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.clearlogs_desc", langCode), &gotgbot.SendMessageOpts{
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
	langCode := I.LangCodePrefer(ctx.EffectiveUser.Id, ctx.EffectiveUser.LanguageCode)
	_, err = ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.restart_desc", langCode), nil)
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
			proc, findErr := os.FindProcess(os.Getpid())
			if findErr != nil {
				return findErr
			}
			if signalErr := proc.Signal(syscall.SIGTERM); signalErr != nil {
				return signalErr
			}
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
	langCode := I.LangCodePrefer(ctx.EffectiveUser.Id, ctx.EffectiveUser.LanguageCode)
	displayText := I.GetLocalisedString("commands.shutdown_desc", langCode)
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		_, exists := os.LookupEnv("INVOCATION_ID")
		if exists {
			displayText += I.GetLocalisedString("commands.shutdown_desc_systemd", langCode)
		}
	}
	inlineKeyboard := &gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
			{
				{Text: I.GetLocalisedString("general.confirm", langCode), CallbackData: "shutdown_confirm", Style: "danger"},
				{Text: I.GetLocalisedString("general.cancel", langCode), CallbackData: "shutdown_cancel", Style: "primary"},
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
	langCode := I.LangCodePrefer(ctx.EffectiveUser.Id, ctx.EffectiveUser.LanguageCode)
	displayText := I.GetLocalisedString("commands.admin_commands_list", langCode)
	_, err = ctx.EffectiveMessage.Reply(b, displayText, nil)
	log.Log(fmt.Sprintf("User %d triggered /admin", ctx.EffectiveUser.Id), C.LogLevelInfo)
	return err
}

func getCommand(b *gotgbot.Bot, ctx *ext.Context) error {
	langCode := I.LangCodePrefer(ctx.EffectiveUser.Id, ctx.EffectiveUser.LanguageCode)
	if ctx.EffectiveChat.Type == "private" {
		_, err := ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.get_desc_private", langCode), nil)
		return err
	}
	if ctx.EffectiveChat.Type != "group" && ctx.EffectiveChat.Type != "supergroup" {
		return nil // 仅允许在群聊中使用 /get 命令，忽略私聊和频道中的命令
	}
	if ctx.EffectiveMessage.ReplyToMessage == nil || ctx.EffectiveMessage.ReplyToMessage.Sticker == nil {
		_, err := ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.get_desc_nosticker", langCode), nil)
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
		_, err := ctx.EffectiveMessage.Reply(b, fmt.Sprintf(I.GetLocalisedString("general.out_of_quota", langCode), limit), nil)
		return err
	}
	if cf.Subscription.Enabled {
		err := utils.SubscribeCheck(b, ctx.EffectiveUser.Id)
		if err != nil {
			channel := strings.TrimPrefix(cf.Subscription.Channel, "@")
			if errors.Is(err, utils.ErrUserNotSubscribed) {
				displayText := fmt.Sprintf(I.GetLocalisedString("general.subscription_required", langCode), channel, cf.Subscription.Channel)
				_, replyErr := ctx.EffectiveMessage.Reply(b, displayText, &gotgbot.SendMessageOpts{
					ParseMode: "HTML",
				})
				if replyErr != nil {
					return replyErr
				}
				return nil
			}
			displayText := fmt.Sprintf(I.GetLocalisedString("general.subscription_check_failed", langCode), channel, cf.Subscription.Channel)
			_, replyErr := ctx.EffectiveMessage.Reply(b, displayText, &gotgbot.SendMessageOpts{
				ParseMode: "HTML",
			})
			if replyErr != nil {
				return replyErr
			}
		}
	}
	msg, err := ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("general.sending_sticker", langCode), nil)
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
	inlineKeyboard := gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
			{
				{
					Text:  I.GetLocalisedString("commands.get_desc_success_deeplink", langCode),
					Url:   fmt.Sprintf("https://t.me/%s?start=%s", b.Username, sticker.SetName),
					Style: "primary",
				},
			},
		},
	}
	_, _, err = b.EditMessageText(I.GetLocalisedString("commands.get_desc_success", langCode), &gotgbot.EditMessageTextOpts{
		ChatId:      msg.Chat.Id,
		MessageId:   msg.MessageId,
		ReplyMarkup: inlineKeyboard,
	})
	database.Init("usageRecord", ctx.EffectiveUser.Id, map[string]any{"usage": 1})
	return err
}

func donate(b *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveChat.Type != "private" {
		return nil // 仅允许在私聊中使用 /donate 命令，忽略群聊和频道中的命令
	}
	isAdmin, err := checkAdmin(b, ctx, "donate")
	if err != nil {
		return err
	}
	langCode := I.LangCodePrefer(ctx.EffectiveUser.Id, ctx.EffectiveUser.LanguageCode)
	if isAdmin {
		_, err := ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.donate_admin_refuse", langCode), nil)
		return err
	}
	cf, err := config.Init()
	if err != nil {
		return err
	}
	if !cf.Donation.Enabled {
		_, err := ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.donate_disabled", langCode), nil)
		if err != nil {
			return err
		}
		return nil
	}
	arg := ctx.Args()
	price := gotgbot.LabeledPrice{
		Label:  I.GetLocalisedString("commands.donate_price_label", langCode),
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
	description := cf.Donation.Description
	if cf.Donation.BonusEnabled {
		description += fmt.Sprintf(I.GetLocalisedString("commands.donate_desc_muliplier_enabled", langCode), C.DonationBonusMultiplier)
	}
	_, err = b.SendInvoice(ctx.EffectiveUser.Id, cf.Donation.Title, description, payload, "XTR", []gotgbot.LabeledPrice{price}, &gotgbot.SendInvoiceOpts{
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
	langCode := I.LangCodePrefer(ctx.EffectiveUser.Id, ctx.EffectiveUser.LanguageCode)
	if len(donates["donates"].([]map[string]any)) == 0 {
		_, err := ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.getalldonates_notfound", langCode), nil)
		return err
	}
	args := ctx.Args()
	var displayText strings.Builder
	displayText.WriteString(I.GetLocalisedString("commands.getalldonates_desc", langCode))
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
		fmt.Fprintf(&displayText, I.GetLocalisedString("commands.getalldonates_desc_item", langCode), userID, amount, timeFormatted, donate["status"])
		recordLength++
	}
	if recordLength == 0 {
		_, err := ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.getalldonates_no_applicants", langCode), nil)
		return err
	}
	_, err = ctx.EffectiveMessage.Reply(b, displayText.String(), nil)
	return err

}

func refund(b *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveChat.Type != "private" {
		return nil // 仅允许在私聊中使用 /refund 命令，忽略群聊和频道中的命令
	}
	isAdmin, err := checkAdmin(b, ctx, "refund")
	if err != nil {
		return err
	}
	langCode := I.LangCodePrefer(ctx.EffectiveUser.Id, ctx.EffectiveUser.LanguageCode)
	if isAdmin {
		_, err := ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.refund_admin_refuse", langCode), nil)
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
		_, err := ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.refund_norecords", langCode), nil)
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
		_, err := ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.refund_expired", langCode), nil)
		return err
	}
	var displayText strings.Builder
	InlineKeyboard := [][]gotgbot.InlineKeyboardButton{}
	displayText.WriteString(I.GetLocalisedString("commands.refund_desc", langCode))
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
				Text:         fmt.Sprintf(I.GetLocalisedString("commands.refund_callback", langCode), i+1),
				CallbackData: fmt.Sprintf("refund_apply_%s", telegramChargeID),
				Style:        "primary",
			},
		})

		amount, _ := anyToInt64(donation["amount"])
		fmt.Fprintf(&displayText, I.GetLocalisedString("commands.refund_desc_item", langCode), i+1, amount, timeFormatted)
	}
	if len(InlineKeyboard) == 0 {
		_, err := ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.refund_desc_no_applicable", langCode), nil)
		return err
	}

	_, err = ctx.EffectiveMessage.Reply(b, displayText.String(), &gotgbot.SendMessageOpts{
		ReplyMarkup: &gotgbot.InlineKeyboardMarkup{
			InlineKeyboard: InlineKeyboard,
		},
	})
	return err
}

type GitHubRelease struct {
	TagName string `json:"tag_name"`
}

func upgrade(b *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveChat.Type != "private" {
		return nil // 仅允许在私聊中使用 /upgrade 命令，忽略群聊和频道中的命令
	}
	isAdmin, err := checkAdmin(b, ctx, "upgrade")
	if isAdmin != true {
		return nil
	}
	langCode := I.LangCodePrefer(ctx.EffectiveUser.Id, ctx.EffectiveUser.LanguageCode)
	if V.Version == "dev" {
		_, err := ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.upgrade_desc_dev", langCode), nil)
		return err
	}
	if os.Getenv("IN_DOCKER") == "true" {
		_, err := ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.upgrade_desc_docker", langCode), nil)
		return err
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", C.Owner, C.Repo)

	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", url, nil)

	// 重要：GitHub API 要求必须设置 User-Agent
	req.Header.Set("User-Agent", "go-github-release-checker")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, err := ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.upgrade_desc_failed", langCode), nil)
		if err != nil {
			return err
		}
		log.Log("Failed to fetch latest release information from GitHub.", C.LogLevelError)
		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	var release GitHubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		_, err := ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.upgrade_desc_failed", langCode), nil)
		if err != nil {
			return err
		}
		log.Log("Failed to parse latest release information from GitHub.", C.LogLevelError)
		return err
	}
	latestVersion := strings.TrimPrefix(release.TagName, "v")

	if latestVersion == V.Version {
		_, err := ctx.EffectiveMessage.Reply(b, fmt.Sprintf(I.GetLocalisedString("commands.upgrade_desc_latest", langCode), C.RepoURL, V.Version), &gotgbot.SendMessageOpts{
			ParseMode: "HTML",
		})
		return err
	}
	displayText := fmt.Sprintf(I.GetLocalisedString("commands.upgrade_desc_new", langCode), C.RepoURL, latestVersion)
	inlineKeyboard := &gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
			{
				{
					Text:         I.GetLocalisedString("commands.upgrade_confirm", langCode),
					CallbackData: fmt.Sprintf("upgrade_true_%s", latestVersion),
					Style:        "success",
				},
				{
					Text:         I.GetLocalisedString("commands.upgrade_cancel", langCode),
					CallbackData: "upgrade_false",
					Style:        "primary",
				},
			},
		},
	}
	_, err = ctx.EffectiveMessage.Reply(b, displayText, &gotgbot.SendMessageOpts{
		ParseMode:   "HTML",
		ReplyMarkup: inlineKeyboard,
	})
	return err
}

func languages(b *gotgbot.Bot, ctx *ext.Context) error {
	langCode := I.LangCodePrefer(ctx.EffectiveUser.Id, ctx.EffectiveUser.LanguageCode)
	supportedLanguages, _, err := I.GetAllSupportedLanguages()
	if err != nil {
		return err
	}
	displayText := I.GetLocalisedString("commands.languages_desc", langCode)
	inlineKeyboard := &gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{},
	}
	for idx, lang := range supportedLanguages {
		keyIndex := idx + 1
		name := lang[fmt.Sprintf("name_%d", keyIndex)]
		code := lang[fmt.Sprintf("code_%d", keyIndex)]
		if name == "" || code == "" {
			continue
		}
		inlineKeyboard.InlineKeyboard = append(inlineKeyboard.InlineKeyboard, []gotgbot.InlineKeyboardButton{
			{
				Text:         name,
				CallbackData: fmt.Sprintf("setlang_%s", code),
				Style:        "primary",
			},
		})
	}
	_, err = ctx.EffectiveMessage.Reply(b, displayText, &gotgbot.SendMessageOpts{
		ReplyMarkup:    inlineKeyboard,
		ProtectContent: true,
	})
	return err

}
