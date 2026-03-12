package callback

import (
	"fmt"
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

	_, _, err = b.EditMessageText("正在获取贴纸包，请稍候...", &gotgbot.EditMessageTextOpts{
		ChatId:    ctx.EffectiveChat.Id,
		MessageId: ctx.CallbackQuery.Message.GetMessageId(),
	})
	if err != nil {
		return err
	}

	zipPath, err := stickers.GetStickerPack(b, packName, ctx.EffectiveUser.Id)
	if after, ok := strings.CutPrefix(zipPath, "too_many_stickers_"); ok {
		path := after
		parts := strings.Split(path, "_")
		length := parts[0]
		limit := parts[1]
		msg := fmt.Sprintf("贴纸包包含 %s 张贴纸，超过每包限制的 %s 张。", length, limit)
		_, _, _ = b.EditMessageText(msg, &gotgbot.EditMessageTextOpts{
			ChatId:    ctx.EffectiveChat.Id,
			MessageId: ctx.CallbackQuery.Message.GetMessageId(),
		})
		return fmt.Errorf("%s", err.Error())
	}
	if err != nil {
		_, _, _ = b.EditMessageText("获取贴纸包失败，请稍后重试。", &gotgbot.EditMessageTextOpts{
			ChatId:    ctx.EffectiveChat.Id,
			MessageId: ctx.CallbackQuery.Message.GetMessageId(),
		})
		return err
	}
	f, err := os.Open(zipPath)
	if err != nil {
		_, _, _ = b.EditMessageText("获取贴纸包失败，请稍后重试。", &gotgbot.EditMessageTextOpts{
			ChatId:    ctx.EffectiveChat.Id,
			MessageId: ctx.CallbackQuery.Message.GetMessageId(),
		})
		return err
	}
	_, err = b.SendDocument(ctx.EffectiveUser.Id, gotgbot.InputFileByReader(f.Name(), f), nil)
	closeErr := f.Close()
	if err != nil {
		if removeErr := os.Remove(zipPath); removeErr != nil {
			fmt.Printf("failed to remove zip after send error %s: %v\n", zipPath, removeErr)
		}
		_, _, _ = b.EditMessageText("发送贴纸包失败，请稍后重试。", &gotgbot.EditMessageTextOpts{
			ChatId:    ctx.EffectiveChat.Id,
			MessageId: ctx.CallbackQuery.Message.GetMessageId(),
		})
		return err
	}
	if closeErr != nil {
		_, _, _ = b.EditMessageText("发送贴纸包失败，请稍后重试。", &gotgbot.EditMessageTextOpts{
			ChatId:    ctx.EffectiveChat.Id,
			MessageId: ctx.CallbackQuery.Message.GetMessageId(),
		})
		return closeErr
	}

	_, _, _ = b.EditMessageText(
		"贴纸包下载完成！",
		&gotgbot.EditMessageTextOpts{
			ChatId:    ctx.EffectiveChat.Id,
			MessageId: ctx.CallbackQuery.Message.GetMessageId(),
		},
	)
	if err = os.Remove(zipPath); err != nil {
		fmt.Printf("failed to remove zip %s: %v\n", zipPath, err)
	}
	return nil
}
