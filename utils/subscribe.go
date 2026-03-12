package utils

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"libost/sticker_go/config"
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
		return err
	}
	if cf.Channel == "" {
		return fmt.Errorf("channel is empty")
	}

	url := "https://api.telegram.org/bot" + b.Token + "/getChatMember"
	payload := getChatMemberRequest{
		ChatID: cf.Channel,
		UserID: uid,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call getChatMember: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("getChatMember http status: %d", resp.StatusCode)
	}

	var response GetChatMemberResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return err
	}
	if !response.Ok {
		return fmt.Errorf("failed to get chat member: %s (code=%d)", response.Description, response.ErrorCode)
	}

	switch response.Result.Status {
	case "member", "administrator", "creator":
		return nil
	case "restricted":
		if response.Result.IsMember {
			return nil
		}
	}

	return ErrUserNotSubscribed
}
