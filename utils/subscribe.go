package utils

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"libost/sticker_go/config"
	C "libost/sticker_go/constants"
	"libost/sticker_go/log"
	"net/http"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

var ErrUserNotSubscribed = errors.New("user is not subscribed")

type getChatMemberRequest struct {
	ChatID string `json:"chat_id"`
	UserID int64  `json:"user_id"`
}

type chatMemberResult struct {
	Status   string `json:"status"`
	IsMember bool   `json:"is_member"`
}

type GetChatMemberResponse struct {
	Ok          bool             `json:"ok"`
	Result      chatMemberResult `json:"result"`
	Description string           `json:"description"`
	ErrorCode   int              `json:"error_code"`
}

func SubscribeCheck(b *gotgbot.Bot, uid int64) error {
	cf, err := config.Init()
	if err != nil {
		log.Log(fmt.Sprintf("User %d failed to load config for subscription check: %v", uid, err), C.LogLevelError)
		return err
	}
	if cf.Channel == "" {
		log.Log(fmt.Sprintf("User %d failed to check subscription: channel is empty", uid), C.LogLevelError)
		return fmt.Errorf("channel is empty")
	}

	url := "https://api.telegram.org/bot" + b.Token + "/getChatMember"
	payload := getChatMemberRequest{
		ChatID: cf.Channel,
		UserID: uid,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		log.Log(fmt.Sprintf("User %d failed to marshal getChatMember request: %v", uid, err), C.LogLevelError)
		return err
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		log.Log(fmt.Sprintf("User %d failed to create getChatMember request: %v", uid, err), C.LogLevelError)
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Log(fmt.Sprintf("User %d failed to call getChatMember: %v", uid, err), C.LogLevelError)
		return fmt.Errorf("failed to call getChatMember: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Log(fmt.Sprintf("User %d failed to get chat member: http status %d", uid, resp.StatusCode), C.LogLevelError)
		return fmt.Errorf("getChatMember http status: %d", resp.StatusCode)
	}

	var response GetChatMemberResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		log.Log(fmt.Sprintf("User %d failed to decode getChatMember response: %v", uid, err), C.LogLevelError)
		return err
	}
	if !response.Ok {
		log.Log(fmt.Sprintf("User %d failed to get chat member: %s (code=%d)", uid, response.Description, response.ErrorCode), C.LogLevelError)
		return fmt.Errorf("failed to get chat member: %s (code=%d)", response.Description, response.ErrorCode)
	}

	switch response.Result.Status {
	case "member", "administrator", "creator":
		log.Log(fmt.Sprintf("User %d is subscribed to the channel", uid), C.LogLevelInfo)
		return nil
	case "restricted":
		if response.Result.IsMember {
			log.Log(fmt.Sprintf("User %d is a restricted member of the channel", uid), C.LogLevelInfo)
			return nil
		}
	}
	log.Log(fmt.Sprintf("User %d is not subscribed to the channel", uid), C.LogLevelWarn)

	return ErrUserNotSubscribed
}
