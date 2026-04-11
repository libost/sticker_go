package stickers

import (
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/libost/sticker_go/config"
	C "github.com/libost/sticker_go/constants"
	"github.com/libost/sticker_go/database"
	I "github.com/libost/sticker_go/i18n"
	"github.com/libost/sticker_go/log"
	"github.com/libost/sticker_go/utils"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/message"
)

var (
	convertingUsers             sync.Map
	ErrUserConversionInProgress = errors.New("user conversion in progress")
)

func IsUserConverting(uid int64) bool {
	_, converting := convertingUsers.Load(uid)
	return converting
}

func tryStartUserConversion(uid int64) bool {
	_, loaded := convertingUsers.LoadOrStore(uid, struct{}{})
	return !loaded
}

func finishUserConversion(uid int64) {
	convertingUsers.Delete(uid)
}

func AddHandlers(dispatcher *ext.Dispatcher) {
	// 在这里注册处理器，例如：
	dispatcher.AddHandler(handlers.NewMessage(message.Sticker, stickerHandler))
}

func stickerHandler(b *gotgbot.Bot, ctx *ext.Context) error {
	// 处理收到的贴纸消息
	sticker := ctx.EffectiveMessage.Sticker
	if sticker == nil {
		return nil // 不是贴纸消息，忽略
	}
	if ctx.EffectiveChat.Type != "private" {
		return nil // 仅处理私聊中的贴纸消息，忽略群聊和频道中的贴纸
	}
	if config.AppConfig.Misc.SelfUse && ctx.EffectiveUser.Id != config.AppConfig.Misc.OwnerId {
		return nil
	}
	currentUsage, err := database.Init("usage", ctx.EffectiveUser.Id, nil)
	if err != nil {
		return err
	}
	if !currentUsage["exists"].(bool) {
		database.Init("create", ctx.EffectiveUser.Id, nil)
		currentUsage["usage"] = float64(0)
	}
	langCode := I.LangCodePrefer(ctx.EffectiveUser.Id, ctx.EffectiveUser.LanguageCode)
	subErr := utils.SubscribeCheck(b, ctx, ctx.EffectiveUser.Id, langCode)
	if subErr != nil {
		return nil // 用户未订阅，已在 SubscribeCheck 中发送提示消息，直接返回不继续处理
	}
	limit := config.AppConfig.General.Limit
	userGroup, err := database.Init("user_group", ctx.EffectiveUser.Id, nil)
	if err != nil {
		return err
	}
	if userGroup["user_group"] == "sponsor" && config.AppConfig.Donation.BonusEnabled {
		limit = int(float64(config.AppConfig.General.Limit) * C.DonationBonusMultiplier) // 赞助用户的使用限制增加奖励倍数
	}
	if int(currentUsage["usage"].(float64)) >= limit && (int(currentUsage["last_cycle_starts_at"].(float64))+24*3600) >= int(time.Now().Unix()) && !config.AppConfig.Misc.SelfUse {
		displayText := fmt.Sprintf(I.GetLocalisedString("general.out_of_quota", langCode), limit)
		if userGroup["user_group"] != "sponsor" && config.AppConfig.Donation.Enabled && config.AppConfig.Donation.BonusEnabled {
			displayText += I.GetLocalisedString("general.donate_reminder_outofquota", langCode)
		}
		if userGroup["user_group"] == "sponsor" && config.AppConfig.Donation.BonusEnabled {
			displayText += I.GetLocalisedString("general.donated", langCode)
		}
		_, err = ctx.EffectiveMessage.Reply(b, displayText, nil)
		return err
	} else if (int(currentUsage["last_cycle_starts_at"].(float64)) + 24*3600) < int(time.Now().Unix()) {
		database.Init("reset_usage", ctx.EffectiveUser.Id, nil)
	}
	sentMsg, err := ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("stickers.processing", langCode), nil)
	if err != nil {
		return err
	}
	stopAction := make(chan struct{})
	var stopActionOnce sync.Once
	stopActionLoop := func() {
		stopActionOnce.Do(func() {
			close(stopAction)
		})
	}
	go func() {
		_, _ = b.SendChatAction(ctx.EffectiveUser.Id, "upload_document", nil)
		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_, _ = b.SendChatAction(ctx.EffectiveUser.Id, "upload_document", nil)
			case <-stopAction:
				return
			}
		}
	}()
	defer stopActionLoop()
	inlineKeyboard := gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
			{
				{
					Text:         I.GetLocalisedString("stickers.getpack_callback", langCode),
					CallbackData: fmt.Sprintf("get_pack_%s", sticker.SetName),
					Style:        "primary",
				},
			},
		},
	}
	filePath, cleanup, err := GetSticker(b, sticker, ctx.EffectiveUser.Id, config.AppConfig)
	if err != nil {
		if errors.Is(err, ErrUserConversionInProgress) {
			_, _, _ = b.EditMessageText(I.GetLocalisedString("stickers.conversion_in_progress", langCode), &gotgbot.EditMessageTextOpts{
				ChatId:    sentMsg.Chat.Id,
				MessageId: sentMsg.MessageId,
			})
			return nil
		}
		return err
	}
	defer cleanup()
	fileSend, err := os.Open(filePath)
	if err != nil {
		return err
	}
	_, err = b.SendDocument(ctx.EffectiveUser.Id, gotgbot.InputFileByReader(fileSend.Name(), fileSend), &gotgbot.SendDocumentOpts{})
	stopActionLoop()
	if err != nil {
		_ = fileSend.Close()
		return err
	}
	if err = fileSend.Close(); err != nil {
		return err
	}
	displayText := I.GetLocalisedString("stickers.success", langCode)
	if userGroup["user_group"] != "sponsor" && config.AppConfig.Donation.Enabled && !config.AppConfig.Misc.SelfUse {
		n := rand.IntN(10) + 1
		if n <= 2 { // 20% 的概率提示用户支持开发
			displayText += I.GetLocalisedString("general.donate_reminder", langCode)
		}
	}
	database.Init("usageRecord", ctx.EffectiveUser.Id, map[string]any{"usage": 1})
	_, _, _ = b.EditMessageText(displayText, &gotgbot.EditMessageTextOpts{
		ChatId:      sentMsg.Chat.Id,
		MessageId:   sentMsg.MessageId,
		ParseMode:   "HTML",
		ReplyMarkup: inlineKeyboard,
	})
	log.Log(fmt.Sprintf("User %d successfully processed sticker %s", ctx.EffectiveUser.Id, sticker.FileId), C.LogLevelInfo)
	return nil
}

func GetSticker(b *gotgbot.Bot, sticker *gotgbot.Sticker, uid int64, cf *config.Config) (string, func(), error) {
	if sticker == nil {
		return "", func() {}, errors.New("sticker is nil")
	}
	if !tryStartUserConversion(uid) {
		return "", func() {}, ErrUserConversionInProgress
	}
	defer finishUserConversion(uid)

	fileExt, fileExtConverted := getStickerExtensions(sticker, cf.General.TgsSupport)
	cachefilePath := getStickerCachePath(sticker.FileId, fileExt, fileExtConverted, cf.General.TgsSupport)
	if _, err := os.Stat(cachefilePath); err == nil {
		log.Log(fmt.Sprintf("Sticker cached: %s", cachefilePath), C.LogLevelInfo)
		return cachefilePath, func() {}, nil
	} else if !os.IsNotExist(err) {
		return "", func() {}, err
	}

	tempDir := fmt.Sprintf("./%s/%d/", C.CacheDir, uid)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", func() {}, err
	}
	cleanup := func() {
		_ = os.RemoveAll(tempDir)
	}

	rawPath := fmt.Sprintf("%s%s%s", tempDir, sticker.FileId, fileExt)
	if err := downloadStickerFile(b, sticker.FileId, rawPath); err != nil {
		cleanup()
		return "", func() {}, err
	}

	filePath, err := convertStickerFile(sticker, rawPath, tempDir, cf.General.TgsSupport)
	if err != nil {
		cleanup()
		return "", func() {}, err
	}

	if !cf.Cache.Enabled {
		return filePath, cleanup, nil
	}

	if err := os.MkdirAll(C.CacheDir, 0755); err != nil {
		cleanup()
		return "", func() {}, err
	}

	if _, err := os.Stat(cachefilePath); os.IsNotExist(err) {
		if err := os.Rename(filePath, cachefilePath); err != nil {
			cleanup()
			return "", func() {}, err
		}
		log.Log(fmt.Sprintf("Moved converted sticker to cache: %s", cachefilePath), C.LogLevelInfo)
	} else if err != nil {
		cleanup()
		return "", func() {}, err
	}

	return cachefilePath, cleanup, nil
}

func getStickerExtensions(sticker *gotgbot.Sticker, tgsSupport bool) (string, string) {
	switch {
	case sticker.IsAnimated:
		if tgsSupport {
			return ".tgs", ".gif"
		}
		return ".tgs", ".tgs"
	case sticker.IsVideo:
		return ".webm", ".gif"
	default:
		return ".webp", ".png"
	}
}

func getStickerCachePath(fileID string, fileExt string, fileExtConverted string, tgsSupport bool) string {
	if fileExt == ".tgs" && tgsSupport {
		return C.CacheDir + fileID + fileExt + fileExtConverted
	}
	return C.CacheDir + fileID + fileExtConverted
}

func downloadStickerFile(b *gotgbot.Bot, fileID string, outputPath string) error {
	file, err := b.GetFile(fileID, &gotgbot.GetFileOpts{})
	if err != nil {
		return err
	}

	downloadURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", b.Token, file.FilePath)
	resp, err := http.Get(downloadURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func convertStickerFile(sticker *gotgbot.Sticker, rawPath string, tempDir string, tgsSupport bool) (string, error) {
	fileExt, _ := getStickerExtensions(sticker, tgsSupport)
	filePath := rawPath

	switch fileExt {
	case ".webp":
		var err error
		filePath, err = utils.DecodeWebPToPNG(rawPath)
		if err != nil {
			return "", err
		}
		log.Log(fmt.Sprintf("Sticker saved as PNG: %s", filePath), C.LogLevelInfo)
	case ".webm":
		var err error
		filePath, err = utils.DecodeWebMToGIF(rawPath)
		if err != nil {
			return "", err
		}
		log.Log(fmt.Sprintf("Video sticker saved as GIF: %s", filePath), C.LogLevelInfo)
	case ".tgs":
		if !tgsSupport {
			log.Log(fmt.Sprintf("Animated sticker uses its original file: %s", filePath), C.LogLevelInfo)
			return filePath, nil
		}

		if err := utils.DecodeTgsToGIF(tempDir); err != nil {
			if errors.Is(err, utils.ErrTgsConversionUnsupported) {
				log.Log(fmt.Sprintf("TGS->GIF unsupported for %s, fallback to original TGS: %s", sticker.FileId, rawPath), C.LogLevelWarn)
				return rawPath, nil
			}
			return "", err
		}

		filePath = tempDir + sticker.FileId + ".tgs" + ".gif"
		if _, statErr := os.Stat(filePath); os.IsNotExist(statErr) {
			log.Log(fmt.Sprintf("Converted GIF not found after TGS conversion for %s, fallback to original TGS: %s", sticker.FileId, rawPath), C.LogLevelWarn)
			return rawPath, nil
		} else if statErr != nil {
			return "", statErr
		}
		_ = os.Remove(tempDir + sticker.FileId + ".json")
		log.Log(fmt.Sprintf("Animated sticker converted to GIF: %s", filePath), C.LogLevelInfo)
	}

	if filePath != rawPath {
		_ = os.Remove(rawPath)
	}

	return filePath, nil
}
