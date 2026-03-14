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
	var fileExt string
	var fileExtConverted string
	switch true {
	case sticker.IsAnimated:
		fileExt = ".tgs"
		if cf.General.TgsSupport {
			fileExtConverted = ".gif"
		} else {
			fileExtConverted = ".tgs"
		}
	case sticker.IsVideo:
		// 处理视频贴纸
		fileExt = ".webm"
		fileExtConverted = ".gif"
	default:
		// 处理普通贴纸
		fileExt = ".webp"
		fileExtConverted = ".png"
	}
	var cachefilePath string
	if fileExt == ".tgs" && cf.General.TgsSupport {
		// 对于 TGS 文件，如果启用了 TGS 支持，先检查转换后的 GIF 是否存在
		cachefilePath = C.CacheDir + sticker.FileId + fileExt + fileExtConverted
	} else {
		// 对于其他文件类型，直接使用原始文件路径
		cachefilePath = C.CacheDir + sticker.FileId + fileExtConverted
	}
	_, err = os.Stat(cachefilePath)
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
	if err == nil {
		log.Log(fmt.Sprintf("Sticker cached: %s", cachefilePath), C.LogLevelInfo)
		fileExist, err := os.Open(cachefilePath)
		if err != nil {
			return err
		}
		defer fileExist.Close()
		_, err = b.SendDocument(ctx.EffectiveUser.Id, gotgbot.InputFileByReader(fileExist.Name(), fileExist), &gotgbot.SendDocumentOpts{})
		if err != nil {
			return err
		}
		database.Init("usageRecord", ctx.EffectiveUser.Id, map[string]any{"usage": 1})
		_, _, _ = b.EditMessageText("处理完成！", &gotgbot.EditMessageTextOpts{
			ChatId:      sentMsg.Chat.Id,
			MessageId:   sentMsg.MessageId,
			ReplyMarkup: inlineKeyboard,
		})
		return nil
	}
	file, err := b.GetFile(sticker.FileId, &gotgbot.GetFileOpts{})
	if err != nil {
		return err
	}
	// 在这里可以处理贴纸文件，例如下载或上传
	downloadUrl := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", b.Token, file.FilePath)
	resp, err := http.Get(downloadUrl)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// 将文件保存到本地
	out, err := os.Create(fmt.Sprintf("%s%s%s", C.CacheDir, sticker.FileId, fileExt))
	if err != nil {
		return err
	}
	_, err = io.Copy(out, resp.Body)
	out.Close()
	if err != nil {
		return err
	}
	var filePath string
	switch fileExt {
	case ".webp":
		filePath, err = utils.DecodeWebPToPNG(sticker.FileId)
		if err != nil {
			return err
		}
		log.Log(fmt.Sprintf("Sticker saved as PNG: %s", filePath), C.LogLevelInfo)
	case ".webm":
		filePath, err = utils.DecodeWebMToGIF(sticker.FileId)
		if err != nil {
			return err
		}
		log.Log(fmt.Sprintf("Video sticker saved as GIF: %s", filePath), C.LogLevelInfo)
	default:
		if cf.General.TgsSupport {
			err = utils.DecodeTgsToGIF(C.CacheDir)
			if err != nil {
				if errors.Is(err, utils.ErrTgsConversionUnsupported) {
					filePath = C.CacheDir + sticker.FileId + ".tgs"
					log.Log(fmt.Sprintf("TGS->GIF unsupported for %s, fallback to original TGS: %s", sticker.FileId, filePath), C.LogLevelWarn)
				} else {
					return err
				}
			} else {
				filePath = C.CacheDir + sticker.FileId + ".tgs" + ".gif"
				os.Remove(C.CacheDir + sticker.FileId + ".json")
				log.Log(fmt.Sprintf("Animated sticker converted to GIF: %s", filePath), C.LogLevelInfo)
			}
		} else {
			filePath = C.CacheDir + sticker.FileId + fileExt
			log.Log(fmt.Sprintf("Animated sticker uses its original file: %s", filePath), C.LogLevelInfo)
		}
	}
	// 仅在转换输出不是原始文件时删除原始文件。
	if filePath != C.CacheDir+sticker.FileId+fileExt {
		os.Remove(C.CacheDir + sticker.FileId + fileExt)
	}
	fileSend, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer fileSend.Close()
	_, err = b.SendDocument(ctx.EffectiveUser.Id, gotgbot.InputFileByReader(fileSend.Name(), fileSend), &gotgbot.SendDocumentOpts{})
	if err != nil {
		return err
	}
	database.Init("usageRecord", ctx.EffectiveUser.Id, map[string]any{"usage": 1})
	_, _, _ = b.EditMessageText("处理完成！", &gotgbot.EditMessageTextOpts{
		ChatId:      sentMsg.Chat.Id,
		MessageId:   sentMsg.MessageId,
		ReplyMarkup: inlineKeyboard,
	})
	if !cf.Cache.Enabled {
		os.Remove(filePath) // 如果缓存未启用，处理完成后删除文件
		log.Log(fmt.Sprintf("Cache disabled, removed file: %s", filePath), C.LogLevelInfo)
	}
	log.Log(fmt.Sprintf("User %d successfully processed sticker %s", ctx.EffectiveUser.Id, sticker.FileId), C.LogLevelInfo)
	return nil
}
