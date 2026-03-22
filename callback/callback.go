package callback

import (
	"bufio"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/libost/sticker_go/config"
	C "github.com/libost/sticker_go/constants"
	"github.com/libost/sticker_go/database"
	"github.com/libost/sticker_go/log"
	"github.com/libost/sticker_go/stickers"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/callbackquery"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/message"
	"github.com/minio/selfupdate"
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
	dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("refund_apply_"), refundApplyHandler))
	dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("upgrade_"), upgradeHandler))
	dispatcher.AddHandler(handlers.NewPreCheckoutQuery(allPreCheckouts, preCheckoutHandler))
	dispatcher.AddHandler(handlers.NewMessage(message.SuccessfulPayment, successHandler))
}

func allPreCheckouts(pq *gotgbot.PreCheckoutQuery) bool {
	return true
}

func preCheckoutHandler(b *gotgbot.Bot, ctx *ext.Context) error {
	pq := ctx.PreCheckoutQuery
	cf, err := config.Init()
	if !cf.Donation.Enabled {
		log.Log(fmt.Sprintf("User %d attempted to make a donation but the donation feature is disabled in config", pq.From.Id), C.LogLevelWarn)
		_, err = b.AnswerPreCheckoutQuery(pq.Id, false, &gotgbot.AnswerPreCheckoutQueryOpts{
			ErrorMessage: "捐赠功能已关闭，暂时无法接受捐赠。",
		})
		return err
	}
	_, err = b.AnswerPreCheckoutQuery(pq.Id, true, nil)
	return err
}

func successHandler(b *gotgbot.Bot, ctx *ext.Context) error {
	sp := ctx.Message.SuccessfulPayment
	// log.Printf("支付成功，订单ID: %s, 金额: %d %s", sp.InvoicePayload, sp.TotalAmount, sp.Currency)
	_, dbErr := database.Init("donateSuccess", ctx.EffectiveUser.Id, map[string]any{
		"amount":             sp.TotalAmount,
		"payload":            sp.InvoicePayload,
		"telegram_charge_id": sp.TelegramPaymentChargeId,
		"provider_charge_id": sp.ProviderPaymentChargeId,
	})
	if dbErr != nil {
		log.Log(fmt.Sprintf("User %d donateSuccess persistence failed: %v", ctx.EffectiveUser.Id, dbErr), C.LogLevelError)
		return dbErr
	}
	_, err := b.SendMessage(ctx.EffectiveChat.Id, fmt.Sprintf("感谢您的支持！我们已经收到您支付的 %d %s。", sp.TotalAmount, sp.Currency), nil)
	if err != nil {
		return err
	}
	return nil
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

func parseSHA256FromChecksums(checksumsData []byte, binaryName string) ([]byte, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(checksumsData)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		hashHex := fields[0]
		fileName := strings.TrimPrefix(fields[1], "*")
		if path.Base(strings.ReplaceAll(fileName, "\\", "/")) != binaryName {
			continue
		}

		hash, err := hex.DecodeString(hashHex)
		if err != nil {
			return nil, fmt.Errorf("invalid SHA256 hex for %s: %w", binaryName, err)
		}
		return hash, nil
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return nil, fmt.Errorf("checksum for %s not found", binaryName)
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
	cf, err := config.Init()
	if err != nil {
		log.Log(fmt.Sprintf("User %d failed to load config after downloading sticker pack: %v", ctx.EffectiveUser.Id, err), C.LogLevelError)
		return err
	}
	usergroup, err := database.Init("user_group", ctx.EffectiveUser.Id, nil)
	if err != nil {
		log.Log(fmt.Sprintf("User %d failed to retrieve user group after downloading sticker pack: %v", ctx.EffectiveUser.Id, err), C.LogLevelError)
		return err
	}
	displayText := "✅ 贴纸包下载完成！"
	if usergroup["user_group"] != "sponsor" && cf.Donation.Enabled {
		n := rand.IntN(10) + 1
		if n <= 2 { // 20% 的概率提示用户支持开发
			displayText += "\n <blockquote>如果你喜欢这个项目，欢迎使用命令 /donate 支持开发</blockquote>"
		}
	}
	_, _, _ = b.EditMessageText(
		displayText,
		&gotgbot.EditMessageTextOpts{
			ChatId:    ctx.EffectiveChat.Id,
			MessageId: ctx.CallbackQuery.Message.GetMessageId(),
			ParseMode: "HTML",
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

func refundApplyHandler(b *gotgbot.Bot, ctx *ext.Context) error {
	callbackData := ctx.CallbackQuery.Data
	telegramPaymentChargeID := strings.TrimPrefix(callbackData, "refund_apply_")
	// 先回应回调查询，避免客户端超时
	_, err := ctx.CallbackQuery.Answer(b, nil)
	if err != nil {
		return err
	}
	ok, err := b.RefundStarPayment(ctx.EffectiveUser.Id, telegramPaymentChargeID, &gotgbot.RefundStarPaymentOpts{})
	if err != nil {
		log.Log(fmt.Sprintf("User %d failed to apply for refund: %v", ctx.EffectiveUser.Id, err), C.LogLevelError)
		_, _, _ = b.EditMessageText("申请退款失败，请稍后重试。", &gotgbot.EditMessageTextOpts{
			ChatId:    ctx.EffectiveChat.Id,
			MessageId: ctx.CallbackQuery.Message.GetMessageId(),
		})
		return err
	}
	if ok {
		log.Log(fmt.Sprintf("User %d successfully applied for refund", ctx.EffectiveUser.Id), C.LogLevelInfo)
		_, _, _ = b.EditMessageText("退款申请已提交！请等待审核结果。", &gotgbot.EditMessageTextOpts{
			ChatId:    ctx.EffectiveChat.Id,
			MessageId: ctx.CallbackQuery.Message.GetMessageId(),
		})
		database.Init("refund", ctx.EffectiveUser.Id, map[string]any{
			"telegram_charge_id": telegramPaymentChargeID,
		})
	} else {
		log.Log(fmt.Sprintf("User %d failed to apply for refund: unknown error", ctx.EffectiveUser.Id), C.LogLevelError)
		_, _, _ = b.EditMessageText("申请退款失败，请稍后重试。", &gotgbot.EditMessageTextOpts{
			ChatId:    ctx.EffectiveChat.Id,
			MessageId: ctx.CallbackQuery.Message.GetMessageId(),
		})
	}
	return nil
}

func upgradeHandler(b *gotgbot.Bot, ctx *ext.Context) error {
	callbackData := ctx.CallbackQuery.Data
	// 先回应回调查询，避免客户端超时
	_, err := ctx.CallbackQuery.Answer(b, nil)
	if err != nil {
		return err
	}
	if callbackData == "upgrade_false" {
		log.Log(fmt.Sprintf("User %d cancelled upgrade", ctx.EffectiveUser.Id), C.LogLevelInfo)
		_, _, _ = b.EditMessageText("已取消更新。", &gotgbot.EditMessageTextOpts{
			ChatId:    ctx.EffectiveChat.Id,
			MessageId: ctx.CallbackQuery.Message.GetMessageId(),
		})
		return nil
	}
	_, _, err = b.EditMessageText("正在更新，请稍候...", &gotgbot.EditMessageTextOpts{
		ChatId:    ctx.EffectiveChat.Id,
		MessageId: ctx.CallbackQuery.Message.GetMessageId(),
	})
	if err != nil {
		return err
	}
	var execname string
	switch runtime.GOOS {
	case "windows":
		execname = "sticker_go_windows.exe"
	case "darwin":
		execname = "sticker_go_macos"
	case "linux":
		execname = "sticker_go_linux"
	}
	currentPath, err := os.Executable()
	if err != nil {
		log.Log(fmt.Sprintf("User %d failed to get executable path before update: %v", ctx.EffectiveUser.Id, err), C.LogLevelError)
		_, _, _ = b.EditMessageText("更新失败：无法确定当前程序路径。", &gotgbot.EditMessageTextOpts{
			ChatId:    ctx.EffectiveChat.Id,
			MessageId: ctx.CallbackQuery.Message.GetMessageId(),
		})
		return err
	}
	restartPath := resolveRestartExecutablePath(currentPath)
	downloadURL := fmt.Sprintf("https://github.com/%s/%s/releases/latest/download/%s", C.Owner, C.Repo, execname)
	resp, err := http.Get(downloadURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Log(fmt.Sprintf("User %d failed to download latest release asset: HTTP %d", ctx.EffectiveUser.Id, resp.StatusCode), C.LogLevelError)
		_, _, _ = b.EditMessageText("下载更新失败，请稍后重试。", &gotgbot.EditMessageTextOpts{
			ChatId:    ctx.EffectiveChat.Id,
			MessageId: ctx.CallbackQuery.Message.GetMessageId(),
		})
		return fmt.Errorf("failed to download latest release asset: HTTP %d", resp.StatusCode)
	}
	checkSumURL := fmt.Sprintf("https://github.com/%s/%s/releases/latest/download/checksums.txt", C.Owner, C.Repo)
	checkSumResp, err := http.Get(checkSumURL)
	if err != nil {
		log.Log(fmt.Sprintf("User %d failed to download checksums file: %v", ctx.EffectiveUser.Id, err), C.LogLevelError)
		_, _, _ = b.EditMessageText("下载更新失败，请稍后重试。", &gotgbot.EditMessageTextOpts{
			ChatId:    ctx.EffectiveChat.Id,
			MessageId: ctx.CallbackQuery.Message.GetMessageId(),
		})
		return err
	}
	defer checkSumResp.Body.Close()
	if checkSumResp.StatusCode != http.StatusOK {
		log.Log(fmt.Sprintf("User %d failed to download checksums file: HTTP %d", ctx.EffectiveUser.Id, checkSumResp.StatusCode), C.LogLevelError)
		_, _, _ = b.EditMessageText("下载更新失败，请稍后重试。", &gotgbot.EditMessageTextOpts{
			ChatId:    ctx.EffectiveChat.Id,
			MessageId: ctx.CallbackQuery.Message.GetMessageId(),
		})
		return fmt.Errorf("failed to download checksums file: HTTP %d", checkSumResp.StatusCode)
	}
	checksumsData, err := io.ReadAll(checkSumResp.Body)
	if err != nil {
		log.Log(fmt.Sprintf("User %d failed to read checksums file: %v", ctx.EffectiveUser.Id, err), C.LogLevelError)
		_, _, _ = b.EditMessageText("下载更新失败，请稍后重试。", &gotgbot.EditMessageTextOpts{
			ChatId:    ctx.EffectiveChat.Id,
			MessageId: ctx.CallbackQuery.Message.GetMessageId(),
		})
		return err
	}

	checksum, err := parseSHA256FromChecksums(checksumsData, execname)
	if err != nil {
		log.Log(fmt.Sprintf("User %d failed to parse checksum for %s: %v", ctx.EffectiveUser.Id, execname, err), C.LogLevelError)
		_, _, _ = b.EditMessageText("下载更新失败：未找到对应文件的校验值。", &gotgbot.EditMessageTextOpts{
			ChatId:    ctx.EffectiveChat.Id,
			MessageId: ctx.CallbackQuery.Message.GetMessageId(),
		})
		return err
	}

	err = selfupdate.Apply(resp.Body, selfupdate.Options{Checksum: checksum})
	if err != nil {
		log.Log(fmt.Sprintf("User %d failed to apply update: %v", ctx.EffectiveUser.Id, err), C.LogLevelError)
		_, _, _ = b.EditMessageText("应用更新失败，请稍后重试。", &gotgbot.EditMessageTextOpts{
			ChatId:    ctx.EffectiveChat.Id,
			MessageId: ctx.CallbackQuery.Message.GetMessageId(),
		})
		return err
	}
	log.Log(fmt.Sprintf("User %d successfully updated the bot", ctx.EffectiveUser.Id), C.LogLevelInfo)
	_, _, _ = b.EditMessageText("更新成功！正在重启...", &gotgbot.EditMessageTextOpts{
		ChatId:    ctx.EffectiveChat.Id,
		MessageId: ctx.CallbackQuery.Message.GetMessageId(),
	})
	_, exists := os.LookupEnv("INVOCATION_ID")
	if exists {
		os.Exit(0)
		return nil
	}
	cmd := exec.Command(restartPath, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = os.Environ()
	err = cmd.Start()
	if err != nil {
		log.Log(fmt.Sprintf("User %d failed to restart the bot after update: %v", ctx.EffectiveUser.Id, err), C.LogLevelError)
		_, _, _ = b.EditMessageText("重启失败，请手动重启机器人。", &gotgbot.EditMessageTextOpts{
			ChatId:    ctx.EffectiveChat.Id,
			MessageId: ctx.CallbackQuery.Message.GetMessageId(),
		})
		return err
	}
	os.Exit(0)
	return nil
}

func resolveRestartExecutablePath(execPath string) string {
	base := filepath.Base(execPath)

	// minio/selfupdate may rename the running binary to a hidden backup like ".binary.old".
	if strings.HasPrefix(base, ".") && strings.HasSuffix(base, ".old") {
		newBase := strings.TrimSuffix(strings.TrimPrefix(base, "."), ".old")
		if newBase != "" {
			return filepath.Join(filepath.Dir(execPath), newBase)
		}
	}

	// Fallback for non-hidden backups such as "binary.old".
	if before, ok := strings.CutSuffix(base, ".old"); ok {
		newBase := before
		if newBase != "" {
			return filepath.Join(filepath.Dir(execPath), newBase)
		}
	}

	return execPath
}
