package utils

import (
	"errors"
	"fmt"
	"strings"

	"github.com/libost/sticker_go/config"
	C "github.com/libost/sticker_go/constants"
	I "github.com/libost/sticker_go/i18n"
	"github.com/libost/sticker_go/log"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

var ErrUserNotSubscribed = errors.New("user is not subscribed")

func SubscribeCheck(b *gotgbot.Bot, ctx *ext.Context, uid int64, langCode string) error {
	cf := config.AppConfig
	if !cf.Subscription.Enabled {
		return nil // 订阅检查未启用，直接通过
	}
	result, err := b.GetChatMember(0, uid, &gotgbot.GetChatMemberOpts{
		RequestOpts: &gotgbot.RequestOpts{
			OverrideParams: map[string]any{
				"chat_id": cf.Subscription.Channel,
			},
		},
	})
	if err != nil {
		log.Log(fmt.Sprintf("User %d failed to check subscription: %v", uid, err), C.LogLevelError)
		channel := strings.TrimPrefix(cf.Subscription.Channel, "@")
		displayText := fmt.Sprintf(I.GetLocalisedString("general.subscription_check_failed", langCode), channel, cf.Subscription.Channel)
		_, replyErr := ctx.EffectiveMessage.Reply(b, displayText, &gotgbot.SendMessageOpts{
			ParseMode: "HTML",
		})
		if replyErr != nil {
			log.Log(fmt.Sprintf("User %d failed to send subscription message: %v", uid, replyErr), C.LogLevelError)
		}
		return err
	}
	member := result.MergeChatMember()
	if member.Status != "left" && member.Status != "kicked" {
		return nil // 用户已订阅
	}
	channel := strings.TrimPrefix(cf.Subscription.Channel, "@")
	displayText := fmt.Sprintf(I.GetLocalisedString("general.subscription_required", langCode), channel, cf.Subscription.Channel)
	_, replyErr := ctx.EffectiveMessage.Reply(b, displayText, &gotgbot.SendMessageOpts{
		ParseMode: "HTML",
	})
	if replyErr != nil {
		log.Log(fmt.Sprintf("User %d failed to send subscription check failed message: %v", uid, replyErr), C.LogLevelError)
	}
	return ErrUserNotSubscribed
}
