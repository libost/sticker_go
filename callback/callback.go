package callback

import (
	"os"
	"strings"

	"libost/sticker_go/stickers"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/callbackquery"
)

func AddHandlers(dispatcher *ext.Dispatcher) {
	dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("get_pack_"), getPackHandler))
}

func getPackHandler(b *gotgbot.Bot, ctx *ext.Context) error {
	callbackData := ctx.CallbackQuery.Data
	packName := strings.TrimPrefix(callbackData, "get_pack_")

	// 先回应回调查询，避免客户端超时
	_, err := ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{
		Text: "正在获取贴纸包，请稍候...",
	})
	if err != nil {
		return err
	}

	sentMsg, err := b.SendMessage(ctx.EffectiveUser.Id, "正在下载整套贴纸包，请稍候...", nil)
	if err != nil {
		return err
	}

	zipPath, err := stickers.GetStickerPack(b, packName)
	if err != nil {
		_, _, _ = b.EditMessageText("获取贴纸包失败，请稍后重试。", &gotgbot.EditMessageTextOpts{
			ChatId:    sentMsg.Chat.Id,
			MessageId: sentMsg.MessageId,
		})
		return err
	}
	f, err := os.Open(zipPath)
	if err != nil {
		_, _, _ = b.EditMessageText("获取贴纸包失败，请稍后重试。", &gotgbot.EditMessageTextOpts{
			ChatId:    sentMsg.Chat.Id,
			MessageId: sentMsg.MessageId,
		})
		return err
	}
	defer f.Close()
	_, err = b.SendDocument(ctx.EffectiveUser.Id, gotgbot.InputFileByReader(f.Name(), f), nil)
	if err != nil {
		_, _, _ = b.EditMessageText("发送贴纸包失败，请稍后重试。", &gotgbot.EditMessageTextOpts{
			ChatId:    sentMsg.Chat.Id,
			MessageId: sentMsg.MessageId,
		})
		return err
	}

	_, _, _ = b.EditMessageText(
		"贴纸包下载完成！",
		&gotgbot.EditMessageTextOpts{
			ChatId:    sentMsg.Chat.Id,
			MessageId: sentMsg.MessageId,
		},
	)
	err = os.Remove(zipPath)
	return nil
}
