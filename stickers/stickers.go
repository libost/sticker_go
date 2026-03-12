package stickers

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"libost/sticker_go/config"
	C "libost/sticker_go/constants"
	"libost/sticker_go/database"
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
	if cf.SubToggle {
		err := utils.SubscribeCheck(b, ctx.EffectiveUser.Id)
		if err != nil {
			if errors.Is(err, utils.ErrUserNotSubscribed) {
				_, replyErr := ctx.EffectiveMessage.Reply(b, "请先订阅频道以使用此功能。", nil)
				if replyErr != nil {
					return replyErr
				}
				return nil
			}
			_, replyErr := ctx.EffectiveMessage.Reply(b, "订阅检查失败，请稍后重试。", nil)
			if replyErr != nil {
				return replyErr
			}
			return err
		}
	}
	limit := cf.Limit
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
		fileExt, fileExtConverted = ".tgs", ".tgs"
	case sticker.IsVideo:
		// 处理视频贴纸
		fileExt = ".webm"
		fileExtConverted = ".gif"
	default:
		// 处理普通贴纸
		fileExt = ".webp"
		fileExtConverted = ".png"
	}
	_, err = os.Stat(C.CacheDir + sticker.FileId + fileExtConverted)
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
		fmt.Printf("贴纸已缓存: %s\n", C.CacheDir+sticker.FileId+fileExtConverted)
		fileExist, err := os.Open(C.CacheDir + sticker.FileId + fileExtConverted)
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
		fmt.Printf("贴纸已保存为 PNG: %s\n", filePath)
	case ".webm":
		filePath, err = utils.DecodeWebMToGIF(sticker.FileId)
		if err != nil {
			return err
		}
		fmt.Printf("视频贴纸已保存为 GIF: %s\n", filePath)
	default:
		// .tgs 动画贴纸：直接发送原始文件
		filePath = C.CacheDir + sticker.FileId + fileExt
		fmt.Printf("贴纸已保存: %s\n", filePath)
	}
	// 仅在已转换格式时删除原始文件（.tgs 直接发送，无需删除）
	if fileExt != fileExtConverted {
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
	return nil
}
