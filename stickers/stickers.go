package stickers

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"libost/sticker_go/config"
	C "libost/sticker_go/constants"
	"libost/sticker_go/database"
	"libost/sticker_go/log"
	"libost/sticker_go/utils"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/message"
)

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
	currentUsage, err := database.Init("usage", ctx.EffectiveUser.Id, nil)
	if err != nil {
		return err
	}
	if !currentUsage["exists"].(bool) {
		database.Init("create", ctx.EffectiveUser.Id, nil)
		currentUsage["usage"] = float64(0)
	}
	cf, err := config.Init()
	if err != nil {
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
			return err
		}
	}
	limit := cf.General.Limit
	if int(currentUsage["usage"].(float64)) >= limit {
		_, err = ctx.EffectiveMessage.Reply(b, fmt.Sprintf("你已达到使用限制，每周期最多使用 %d 次。", limit), nil)
		return err
	}
	sentMsg, err := ctx.EffectiveMessage.Reply(b, "正在处理你的贴纸，请稍候...", nil)
	if err != nil {
		return err
	}
	inlineKeyboard := gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
			{
				{
					Text:         "获取整套贴纸包",
					CallbackData: fmt.Sprintf("get_pack_%s", sticker.SetName),
					// 这里的 CallbackData 可以用来在回调查询处理器中识别用户点击了哪个按钮
					// 你需要在回调查询处理器中解析这个数据，并根据 sticker.SetName 来获取并发送整套贴纸包
				},
			},
		},
	}
	filePath, cleanup, err := GetSticker(b, sticker, ctx.EffectiveUser.Id, cf)
	if err != nil {
		return err
	}
	defer cleanup()
	fileSend, err := os.Open(filePath)
	if err != nil {
		return err
	}
	_, err = b.SendDocument(ctx.EffectiveUser.Id, gotgbot.InputFileByReader(fileSend.Name(), fileSend), &gotgbot.SendDocumentOpts{})
	if err != nil {
		_ = fileSend.Close()
		return err
	}
	if err = fileSend.Close(); err != nil {
		return err
	}
	database.Init("usageRecord", ctx.EffectiveUser.Id, map[string]any{"usage": 1})
	_, _, _ = b.EditMessageText("处理完成！", &gotgbot.EditMessageTextOpts{
		ChatId:      sentMsg.Chat.Id,
		MessageId:   sentMsg.MessageId,
		ReplyMarkup: inlineKeyboard,
	})
	log.Log(fmt.Sprintf("User %d successfully processed sticker %s", ctx.EffectiveUser.Id, sticker.FileId), C.LogLevelInfo)
	return nil
}

func GetSticker(b *gotgbot.Bot, sticker *gotgbot.Sticker, uid int64, cf *config.Config) (string, func(), error) {
	if sticker == nil {
		return "", func() {}, errors.New("sticker is nil")
	}

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
		_ = os.Remove(tempDir + sticker.FileId + ".json")
		log.Log(fmt.Sprintf("Animated sticker converted to GIF: %s", filePath), C.LogLevelInfo)
	}

	if filePath != rawPath {
		_ = os.Remove(rawPath)
	}

	return filePath, nil
}
