package dm

import (
	"errors"
	"fmt"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/message"
	"github.com/libost/sticker_go/callback"
	C "github.com/libost/sticker_go/constants"
	I "github.com/libost/sticker_go/i18n"
	"github.com/libost/sticker_go/log"
	"github.com/libost/sticker_go/stickers"
	"github.com/libost/sticker_go/utils"
)

func AddHandlers(dispatcher *ext.Dispatcher) {
	dispatcher.AddHandler(handlers.NewMessage(message.Text, textPrefixHandler))
}

func textPrefixHandler(b *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveChat.Type != "private" {
		return nil
	}
	text := ctx.EffectiveMessage.Text
	langCode := I.LangCodePrefer(ctx.EffectiveUser.Id, ctx.EffectiveUser.LanguageCode)
	switch true {
	case strings.HasPrefix(text, "https://t.me/addstickers/"), strings.HasPrefix(text, "t.me/addstickers/"):
		return stickerLink(b, ctx, text, langCode)
	}
	return nil
}

func stickerLink(b *gotgbot.Bot, ctx *ext.Context, text string, langCode string) error {
	log.Log(fmt.Sprintf("User %d triggered text handler with text: %s", ctx.EffectiveUser.Id, text), C.LogLevelInfo)
	subErr := utils.SubscribeCheck(b, ctx, ctx.EffectiveUser.Id, langCode)
	if subErr != nil {
		return nil
	}
	if strings.HasPrefix(text, "t.me/addstickers/") {
		text = "https://" + text
	}
	packName := strings.TrimPrefix(text, "https://t.me/addstickers/")
	msg, err := ctx.EffectiveMessage.Reply(b, I.GetLocalisedString("commands.get_desc_processing", langCode), nil)
	if err != nil {
		return err
	}
	err = callback.GetPack(b, ctx, packName, langCode, msg.GetMessageId())
	var stickerPackLimitErr *stickers.StickerPackLimitError
	if err != nil && !errors.Is(err, stickerPackLimitErr) && !errors.Is(err, C.ErrOutofQuota) {
		_, _, _ = b.EditMessageText(I.GetLocalisedString("dm.pack_not_found", langCode), &gotgbot.EditMessageTextOpts{
			ChatId:    ctx.EffectiveChat.Id,
			MessageId: msg.GetMessageId(),
		},
		)
	}
	return nil
}
