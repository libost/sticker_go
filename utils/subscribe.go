package utils

import (
	"errors"
	"fmt"

	"github.com/libost/sticker_go/config"
	C "github.com/libost/sticker_go/constants"
	"github.com/libost/sticker_go/log"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

var ErrUserNotSubscribed = errors.New("user is not subscribed")

func SubscribeCheck(b *gotgbot.Bot, uid int64) error {
	cf, err := config.Init()
	if err != nil {
		log.Log(fmt.Sprintf("User %d failed to load config for subscription check: %v", uid, err), C.LogLevelError)
		return err
	}
	if cf.Subscription.Channel == "" {
		log.Log(fmt.Sprintf("User %d failed to check subscription: channel is empty", uid), C.LogLevelError)
		return fmt.Errorf("channel is empty")
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
		return err
	}
	member := result.MergeChatMember()
	if member.Status != "left" && member.Status != "kicked" {
		return nil // 用户已订阅
	}
	return ErrUserNotSubscribed
}
