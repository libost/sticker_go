package callback

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	C "libost/sticker_go/constants"
	"libost/sticker_go/log"
	"libost/sticker_go/stickers"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/callbackquery"
)

const (
	zipSendAttempts     = 3
	zipSendTimeout      = 3 * time.Minute // 按照国内最小的带宽 3Mbps 来计算，50MB 的文件大约需要 2 分钟，这里设置为 3 分钟以提供一些缓冲时间
	zipSendRetryBackoff = 2 * time.Second
)

func AddHandlers(dispatcher *ext.Dispatcher) {
	dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("get_pack_"), getPackHandler))
	dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("clear_logs_"), clearLogsHandler))
	dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("shutdown_"), shutdownHandler))
}

func isRetryableSendDocumentError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "context deadline exceeded") || strings.Contains(errMsg, "timeout")
}

func sendZipDocumentWithRetry(b *gotgbot.Bot, userID int64, zipPath string) error {
	var lastErr error

	for attempt := 1; attempt <= zipSendAttempts; attempt++ {
		f, err := os.Open(zipPath)
		if err != nil {
			return err
		}

		_, err = b.SendDocument(userID, gotgbot.InputFileByReader(f.Name(), f), &gotgbot.SendDocumentOpts{
			RequestOpts: &gotgbot.RequestOpts{
				Timeout: zipSendTimeout,
			},
		})
		closeErr := f.Close()
		if err == nil {
			if closeErr != nil {
				return closeErr
			}
			return nil
		}

		if closeErr != nil {
			log.Log(fmt.Sprintf("User %d failed to close zip file %s after send attempt %d: %v", userID, zipPath, attempt, closeErr), C.LogLevelError)
		}

		lastErr = err
		if attempt == zipSendAttempts || !isRetryableSendDocumentError(err) {
			break
		}

		backoff := zipSendRetryBackoff * time.Duration(1<<(attempt-1))
		log.Log(fmt.Sprintf("Retrying zip send for user %d, file %s, attempt %d/%d after error: %v", userID, zipPath, attempt+1, zipSendAttempts, err), C.LogLevelWarn)
		time.Sleep(backoff)
	}

	return lastErr
}

func removeZipFiles(zipPaths []string) {
	for _, zipPath := range zipPaths {
		if err := os.Remove(zipPath); err != nil {
			fmt.Printf("failed to remove zip %s: %v\n", zipPath, err)
		}
	}
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
	log.Log(fmt.Sprintf("User %d triggered getPackHandler for pack %s", ctx.EffectiveUser.Id, packName), C.LogLevelInfo)

	zipPaths, err := stickers.GetStickerPack(b, packName, ctx.EffectiveUser.Id, ctx.CallbackQuery.Message.GetMessageId())
	var limitErr *stickers.StickerPackLimitError
	if errors.As(err, &limitErr) {
		msg := fmt.Sprintf("贴纸包包含 %d 张贴纸，超过每包限制的 %d 张。", limitErr.PackLength, limitErr.Limit)
		_, _, _ = b.EditMessageText(msg, &gotgbot.EditMessageTextOpts{
			ChatId:    ctx.EffectiveChat.Id,
			MessageId: ctx.CallbackQuery.Message.GetMessageId(),
		})
		log.Log(fmt.Sprintf("User %d attempted to download a sticker pack with too many stickers", ctx.EffectiveUser.Id), C.LogLevelWarn)
		return err
	}
	if err != nil {
		_, _, _ = b.EditMessageText("获取贴纸包失败，请稍后重试。", &gotgbot.EditMessageTextOpts{
			ChatId:    ctx.EffectiveChat.Id,
			MessageId: ctx.CallbackQuery.Message.GetMessageId(),
		})
		log.Log(fmt.Sprintf("User %d failed to download sticker pack %s", ctx.EffectiveUser.Id, packName), C.LogLevelError)
		return err
	}

	if len(zipPaths) > 1 {
		_, _, _ = b.EditMessageText(
			fmt.Sprintf("贴纸包较大，正在分开发送，共 %d 个压缩包。", len(zipPaths)),
			&gotgbot.EditMessageTextOpts{
				ChatId:    ctx.EffectiveChat.Id,
				MessageId: ctx.CallbackQuery.Message.GetMessageId(),
			},
		)
	}

	for _, zipPath := range zipPaths {
		err = sendZipDocumentWithRetry(b, ctx.EffectiveUser.Id, zipPath)
		if err != nil {
			removeZipFiles(zipPaths)
			_, _, _ = b.EditMessageText("发送贴纸包失败，请稍后重试。", &gotgbot.EditMessageTextOpts{
				ChatId:    ctx.EffectiveChat.Id,
				MessageId: ctx.CallbackQuery.Message.GetMessageId(),
			})
			os.RemoveAll(fmt.Sprintf("%s/%d", C.CacheDir, ctx.EffectiveUser.Id))
			return err
		}
	}

	_, _, _ = b.EditMessageText(
		"贴纸包下载完成！",
		&gotgbot.EditMessageTextOpts{
			ChatId:    ctx.EffectiveChat.Id,
			MessageId: ctx.CallbackQuery.Message.GetMessageId(),
		},
	)
	log.Log(fmt.Sprintf("User %d successfully downloaded sticker pack %s", ctx.EffectiveUser.Id, packName), C.LogLevelInfo)
	removeZipFiles(zipPaths)
	os.RemoveAll(fmt.Sprintf("%s/%d", C.CacheDir, ctx.EffectiveUser.Id))
	return nil
}

func clearLogsHandler(b *gotgbot.Bot, ctx *ext.Context) error {
	callbackData := ctx.CallbackQuery.Data
	result := strings.TrimPrefix(callbackData, "clear_logs_")
	// 先回应回调查询，避免客户端超时
	_, err := ctx.CallbackQuery.Answer(b, nil)
	if err != nil {
		return err
	}
	if result == "confirm" {
		logDir := C.LogDir
		err := os.RemoveAll(logDir)
		if err != nil {
			log.Log(fmt.Sprintf("User %d failed to clear logs: %v", ctx.EffectiveUser.Id, err), C.LogLevelError)
			_, _, _ = b.EditMessageText("清除日志失败，请稍后重试。", &gotgbot.EditMessageTextOpts{
				ChatId:    ctx.EffectiveChat.Id,
				MessageId: ctx.CallbackQuery.Message.GetMessageId(),
			})
			return err
		}
		os.Mkdir(logDir, 0755) // 重新创建日志目录
		log.Log(fmt.Sprintf("User %d cleared all logs", ctx.EffectiveUser.Id), C.LogLevelInfo)
		_, _, _ = b.EditMessageText("日志已清除！", &gotgbot.EditMessageTextOpts{
			ChatId:    ctx.EffectiveChat.Id,
			MessageId: ctx.CallbackQuery.Message.GetMessageId(),
		})
		return nil
	}
	_, _, _ = b.EditMessageText("已取消清除日志。", &gotgbot.EditMessageTextOpts{
		ChatId:    ctx.EffectiveChat.Id,
		MessageId: ctx.CallbackQuery.Message.GetMessageId(),
	})
	log.Log(fmt.Sprintf("User %d cancelled log clearing", ctx.EffectiveUser.Id), C.LogLevelInfo)
	return nil
}

func shutdownHandler(b *gotgbot.Bot, ctx *ext.Context) error {
	callbackData := ctx.CallbackQuery.Data
	result := strings.TrimPrefix(callbackData, "shutdown_")
	// 先回应回调查询，避免客户端超时
	_, err := ctx.CallbackQuery.Answer(b, nil)
	if err != nil {
		return err
	}
	if result == "confirm" {
		log.Log(fmt.Sprintf("User %d initiated shutdown", ctx.EffectiveUser.Id), C.LogLevelWarn)
		_, _, _ = b.EditMessageText("正在关闭机器人...", &gotgbot.EditMessageTextOpts{
			ChatId:    ctx.EffectiveChat.Id,
			MessageId: ctx.CallbackQuery.Message.GetMessageId(),
		})
		os.Exit(0)
	} else {
		log.Log(fmt.Sprintf("User %d cancelled shutdown", ctx.EffectiveUser.Id), C.LogLevelInfo)
		_, _, _ = b.EditMessageText("已取消关闭机器人。", &gotgbot.EditMessageTextOpts{
			ChatId:    ctx.EffectiveChat.Id,
			MessageId: ctx.CallbackQuery.Message.GetMessageId(),
		})
	}
	return nil
}
