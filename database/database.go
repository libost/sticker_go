package database

import (
	"database/sql"
	"embed"
	"fmt"
	"sync"
	"time"

	C "github.com/libost/sticker_go/constants"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaFS embed.FS

var (
	db     *sql.DB
	dbOnce sync.Once
	dbErr  error
)

/*
返回data类型示例：
{//init
	"user_id": 123456789,
	"exists": true
}
{//usage
	"user_id": 123456789,
	"exists": true,
	"usage": 100,
	"last_cycle_starts_at": "1800000000",// 这是一个 Unix 时间戳，表示上一个周期的开始时间
}
{//user_group
	"user_id": 123456789,
	"exists": true,
	"user_group": "user/admin/sponsor"
}
{//stats
	"user_id": 123456789,
	"exists": true,
	"stats": {
		"total_users": 1000,
		"total_usage": 500
	}
}
{//create
	"user_id": 123456789,
	"exists": false
}
*/

func getDB() (*sql.DB, error) {
	dbOnce.Do(func() {
		db, dbErr = sql.Open("sqlite", C.DatabaseFile)
		if dbErr != nil {
			return
		}

		schema, err := schemaFS.ReadFile("schema.sql")
		if err != nil {
			dbErr = err
			return
		}

		if _, err := db.Exec(string(schema)); err != nil {
			dbErr = err
			return
		}
	})

	if dbErr != nil {
		return nil, dbErr
	}
	return db, nil
}

func createUserIfNotExists(conn *sql.DB, id int64) error {
	_, err := conn.Exec(
		"INSERT OR IGNORE INTO USERPOOL (user_id, obfu_id) VALUES (?, ?)",
		id,
		fmt.Sprintf("u_%d_%d", id, time.Now().UnixNano()),
	)
	return err
}

func normalizeUsageCycle(conn *sql.DB, id int64) error {
	_, err := conn.Exec(
		`UPDATE USERPOOL
		 SET usage_count = 0,
		     last_cycle_starts_at = unixepoch()
		 WHERE user_id = ?
		   AND unixepoch() - last_cycle_starts_at >= 30 * 24 * 3600`,
		id,
	)
	return err
}

func toUsageInt(other map[string]any) (int, error) {
	if other == nil {
		return 0, fmt.Errorf("missing usage")
	}
	v, ok := other["usage"]
	if !ok {
		return 0, fmt.Errorf("missing usage")
	}
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		return int(n), nil
	default:
		return 0, fmt.Errorf("invalid usage type")
	}
}

func logIntoDonateLogs(conn *sql.DB, id int64, amount int64, payload string) error {
	_, err := conn.Exec(
		"INSERT INTO DONATION_LOGS (user_id, amount, timestamp, payload, telegram_payment_charge_id, provider_payment_charge_id) VALUES (?, ?, ?, ?, ?, ?)",
		id,
		amount,
		time.Now().Unix(),
		payload,
		"pending", // 这里的 Telegram 支付交易 ID 需要在实际处理支付成功的回调时更新
		"pending", // 这里的支付提供商交易 ID 需要在实际处理支付成功的回调时更新
	)
	return err
}

func logIntoDonateLogsSuccess(conn *sql.DB, id int64, payload string, telegramChargeID string, providerChargeID string) error {
	_, err := conn.Exec(
		"UPDATE DONATION_LOGS SET telegram_payment_charge_id = ?, provider_payment_charge_id = ? WHERE payload = ?",
		telegramChargeID,
		providerChargeID,
		payload,
	)
	if err != nil {
		return err
	}
	_, err = conn.Exec(
		"UPDATE USERPOOL SET user_group = 'sponsor' WHERE user_id = ? AND user_group != 'admin'",
		id,
	)
	return err
}

func initCase(id int64, conn *sql.DB) (map[string]any, error) {
	data := map[string]any{"user_id": id, "exists": false}
	// 从这里往下都是兼容性处理，确保即使之前的版本没有 language_code 字段也能正常使用，并且在访问时能够正确返回默认值。
	rows, err := conn.Query("PRAGMA table_info(USERPOOL)")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hasLanguageCode := false
	for rows.Next() {
		var cid int
		var name string
		var colType string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &pk); err != nil {
			return nil, err
		}
		if name == "language_code" {
			hasLanguageCode = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if !hasLanguageCode {
		if _, err := conn.Exec("ALTER TABLE USERPOOL ADD COLUMN language_code TEXT"); err != nil {
			return nil, err
		}
	}

	if id > 0 {
		var languageCode sql.NullString
		err := conn.QueryRow("SELECT language_code FROM USERPOOL WHERE user_id = ?", id).Scan(&languageCode)
		if err == sql.ErrNoRows {
			return data, nil
		}
		if err != nil {
			return nil, err
		}
		if languageCode.Valid && languageCode.String != "" {
			data["language_code"] = languageCode.String
		} else {
			data["language_code"] = "en"
		}
	}
	return data, nil
}

func createCase(id int64, conn *sql.DB) (map[string]any, error) {
	data := map[string]any{"user_id": id, "exists": false}
	if err := createUserIfNotExists(conn, id); err != nil {
		return nil, err
	}
	data["exists"] = true
	return data, nil
}

func usageCase(id int64, conn *sql.DB) (map[string]any, error) {
	data := map[string]any{"user_id": id, "exists": false}
	if err := normalizeUsageCycle(conn, id); err != nil {
		return nil, err
	}
	var usageCount int64
	var lastCycle int64
	err := conn.QueryRow(
		"SELECT usage_count, last_cycle_starts_at FROM USERPOOL WHERE user_id = ?",
		id,
	).Scan(&usageCount, &lastCycle)
	if err == sql.ErrNoRows {
		return data, nil
	}
	if err != nil {
		return nil, err
	}
	data["exists"] = true
	data["usage"] = float64(usageCount)
	data["last_cycle_starts_at"] = float64(lastCycle)
	return data, nil
}

func usageRecordCase(id int64, conn *sql.DB, other map[string]any) (map[string]any, error) {
	data := map[string]any{"user_id": id, "exists": false}
	if err := createUserIfNotExists(conn, id); err != nil {
		return nil, err
	}
	if err := normalizeUsageCycle(conn, id); err != nil {
		return nil, err
	}
	usage, err := toUsageInt(other)
	if err != nil {
		return nil, err
	}
	_, err = conn.Exec(
		"UPDATE USERPOOL SET usage_count = usage_count + ?, total_usage_count = total_usage_count + ? WHERE user_id = ?",
		usage,
		usage,
		id,
	)
	if err != nil {
		return nil, err
	}
	weekday := time.Now().Weekday().String()
	_, err = conn.Exec(
		"INSERT INTO STATISTICS (weekday, daily_usage_count) VALUES (?, ?) ON CONFLICT(weekday) DO UPDATE SET daily_usage_count = daily_usage_count + ?",
		weekday,
		usage,
		usage,
	)
	if err != nil {
		return nil, err
	}
	data["exists"] = true
	return data, nil
}

func userGroupCase(id int64, conn *sql.DB) (map[string]any, error) {
	data := map[string]any{"user_id": id, "exists": true}
	var group string
	err := conn.QueryRow("SELECT user_group FROM USERPOOL WHERE user_id = ?", id).Scan(&group)
	if err == sql.ErrNoRows {
		return data, nil
	}
	if err != nil {
		return nil, err
	}
	data["exists"] = true
	data["user_group"] = group
	return data, nil
}

func statsCase(id int64, conn *sql.DB) (map[string]any, error) {
	data := map[string]any{"user_id": id, "exists": true}
	var totalUsers int64
	var totalUsage int64
	if err := conn.QueryRow("SELECT COUNT(1) FROM USERPOOL").Scan(&totalUsers); err != nil {
		return nil, err
	}
	if err := conn.QueryRow("SELECT COALESCE(SUM(total_usage_count), 0) FROM USERPOOL").Scan(&totalUsage); err != nil {
		return nil, err
	}
	data["stats"] = map[string]any{
		"total_users": float64(totalUsers),
		"total_usage": float64(totalUsage),
	}
	var weeklyUsage int64
	if err := conn.QueryRow("SELECT COALESCE(SUM(daily_usage_count), 0) FROM STATISTICS").Scan(&weeklyUsage); err != nil {
		return nil, err
	}
	data["stats"].(map[string]any)["weekly_usage"] = float64(weeklyUsage)
	return data, nil
}

func setGroupCase(id int64, conn *sql.DB, other map[string]any) (map[string]any, error) {
	data := map[string]any{"user_id": id, "exists": true}
	group, ok := other["group"].(string)
	if !ok {
		return nil, fmt.Errorf("missing group")
	}
	if _, err := conn.Exec("UPDATE USERPOOL SET user_group = ? WHERE user_id = ?", group, id); err != nil {
		return nil, err
	}
	data["exists"] = true
	data["user_group"] = group
	return data, nil
}

func resetUsageCase(id int64, conn *sql.DB) (map[string]any, error) {
	data := map[string]any{"user_id": id, "exists": true}
	if _, err := conn.Exec("UPDATE USERPOOL SET usage_count = 0, last_cycle_starts_at = unixepoch() WHERE user_id = ?", id); err != nil {
		return nil, err
	}
	data["exists"] = true
	data["usage"] = float64(0)
	data["last_cycle_starts_at"] = float64(time.Now().Unix())
	return data, nil
}

func donateInitCase(id int64, conn *sql.DB, other map[string]any) (map[string]any, error) {
	data := map[string]any{"user_id": id, "exists": true}
	amount, ok := other["amount"].(int64)
	if !ok {
		return nil, fmt.Errorf("missing amount")
	}
	payload, ok := other["payload"].(string)
	if !ok {
		return nil, fmt.Errorf("missing payload")
	}
	if err := logIntoDonateLogs(conn, id, amount, payload); err != nil {
		return nil, err
	}
	return data, nil
}

func donateSuccessCase(id int64, conn *sql.DB, other map[string]any) (map[string]any, error) {
	data := map[string]any{"user_id": id, "exists": true}
	payload, ok := other["payload"].(string)
	if !ok {
		return nil, fmt.Errorf("missing payload")
	}
	telegramChargeID, ok := other["telegram_charge_id"].(string)
	if !ok {
		return nil, fmt.Errorf("missing telegram_charge_id")
	}
	providerChargeID, ok := other["provider_charge_id"].(string)
	if !ok {
		return nil, fmt.Errorf("missing provider_charge_id")
	}
	if err := logIntoDonateLogsSuccess(conn, id, payload, telegramChargeID, providerChargeID); err != nil {
		return nil, err
	}
	_, err := conn.Exec(
		"UPDATE DONATION_LOGS SET telegram_payment_charge_id = ?, provider_payment_charge_id = ?, timestamp = ?, status = 'success' WHERE payload = ?",
		telegramChargeID,
		providerChargeID,
		time.Now().Unix(),
		payload,
	)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func refundCase(id int64, conn *sql.DB, other map[string]any) (map[string]any, error) {
	data := map[string]any{"user_id": id, "exists": true}
	telegramChargeID, ok := other["telegram_charge_id"].(string)
	if !ok {
		return nil, fmt.Errorf("missing telegram_charge_id")
	}
	_, err := conn.Exec(
		"UPDATE DONATION_LOGS SET status = 'refunded' WHERE telegram_payment_charge_id = ?",
		telegramChargeID,
	)
	if err != nil {
		return data, err
	}
	// 检查该用户是否仍有成功捐赠；如果没有则将其从 sponsor 降级为 user。
	var successCount int64
	err = conn.QueryRow(
		"SELECT COUNT(1) FROM DONATION_LOGS WHERE user_id = ? AND status = 'success'",
		id,
	).Scan(&successCount)
	if err != nil {
		return data, err
	}
	if successCount == 0 {
		_, err = conn.Exec(
			"UPDATE USERPOOL SET user_group = 'user' WHERE user_id = ? AND user_group = 'sponsor'",
			id,
		)
		return data, err
	}
	return data, nil
}

func getUserDonationsCase(id int64, conn *sql.DB) (map[string]any, error) {
	data := map[string]any{"user_id": id, "exists": true}
	rows, err := conn.Query(
		"SELECT amount, timestamp, payload, telegram_payment_charge_id, provider_payment_charge_id, status FROM DONATION_LOGS WHERE user_id = ? AND status = 'success' ORDER BY timestamp DESC",
		id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	donations := []map[string]any{}
	for rows.Next() {
		var amount int64
		var timestamp int64
		var payload string
		var telegramChargeID string
		var providerChargeID string
		var status string
		if err := rows.Scan(&amount, &timestamp, &payload, &telegramChargeID, &providerChargeID, &status); err != nil {
			return nil, err
		}
		donation := map[string]any{
			"amount":                     amount,
			"timestamp":                  timestamp,
			"payload":                    payload,
			"telegram_payment_charge_id": telegramChargeID,
			"provider_payment_charge_id": providerChargeID,
			"status":                     status,
		}
		donations = append(donations, donation)
	}
	data["donations"] = donations
	return data, nil
}

func getAllDonationsCase(conn *sql.DB) (map[string]any, error) {
	data := map[string]any{}
	rows, err := conn.Query(
		"SELECT user_id, amount, timestamp, payload, telegram_payment_charge_id, provider_payment_charge_id, status FROM DONATION_LOGS ORDER BY timestamp DESC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	donates := []map[string]any{}
	for rows.Next() {
		var userID int64
		var amount int64
		var timestamp int64
		var payload string
		var telegramChargeID string
		var providerChargeID string
		var status string
		if err := rows.Scan(&userID, &amount, &timestamp, &payload, &telegramChargeID, &providerChargeID, &status); err != nil {
			return nil, err
		}
		donate := map[string]any{
			"user_id":                    userID,
			"amount":                     amount,
			"timestamp":                  timestamp,
			"payload":                    payload,
			"telegram_payment_charge_id": telegramChargeID,
			"provider_payment_charge_id": providerChargeID,
			"status":                     status,
		}
		donates = append(donates, donate)
	}
	data["donates"] = donates
	return data, nil
}

func clearWeeklyStatsCase(conn *sql.DB) (map[string]any, error) {
	data := map[string]any{}
	tx, err := conn.Begin()
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec("DELETE FROM STATISTICS")
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	_, err = tx.Exec("INSERT INTO LAST_CLEANUP (id, last_cleanup_at) VALUES (1, unixepoch()) ON CONFLICT(id) DO UPDATE SET last_cleanup_at = unixepoch()")
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return data, nil
}

func getLastCleanupTimeCase(conn *sql.DB) (map[string]any, error) {
	data := map[string]any{}
	var lastCleanup int64
	err := conn.QueryRow("SELECT last_cleanup_at FROM LAST_CLEANUP WHERE id = 1").Scan(&lastCleanup)
	if err == sql.ErrNoRows {
		data["last_cleanup_at"] = float64(0)
		return data, nil
	}
	if err != nil {
		return nil, err
	}
	data["last_cleanup_at"] = float64(lastCleanup)
	return data, nil
}

func languageCodeCase(id int64, conn *sql.DB, other map[string]any) (map[string]any, error) {
	data := map[string]any{"user_id": id, "exists": true}
	requestType := other["type"]
	switch requestType {
	case "get":
		var languageCode sql.NullString
		err := conn.QueryRow("SELECT language_code FROM USERPOOL WHERE user_id = ?", id).Scan(&languageCode)
		if err == sql.ErrNoRows || !languageCode.Valid {
			data["language_exists"] = false
			return data, nil
		}
		if err != nil {
			return nil, err
		}
		data["language_code"] = languageCode.String
		data["language_exists"] = true
		return data, nil
	case "set":
		languageCode, ok := other["language_code"].(string)
		if !ok {
			return nil, fmt.Errorf("missing language_code")
		}
		if _, err := conn.Exec("UPDATE USERPOOL SET language_code = ? WHERE user_id = ?", languageCode, id); err != nil {
			return nil, err
		}
		data["language_code"] = languageCode
		return data, nil
	default:
		return nil, fmt.Errorf("unsupported language_code request type: %s", requestType)
	}
}

func Init(request string, id int64, other map[string]any) (map[string]any, error) {
	conn, err := getDB()
	if err != nil {
		return nil, err
	}

	switch request {
	case "init":
		return initCase(id, conn)
	case "create":
		return createCase(id, conn)
	case "usage":
		return usageCase(id, conn)
	case "usageRecord":
		return usageRecordCase(id, conn, other)
	case "user_group":
		return userGroupCase(id, conn)
	case "stats":
		return statsCase(id, conn)
	case "set_group":
		return setGroupCase(id, conn, other)
	case "reset_usage":
		return resetUsageCase(id, conn)
	case "donateInit":
		return donateInitCase(id, conn, other)
	case "donateSuccess":
		return donateSuccessCase(id, conn, other)
	case "refund":
		return refundCase(id, conn, other)
	case "getUserDonations":
		return getUserDonationsCase(id, conn)
	case "get_all_donates":
		return getAllDonationsCase(conn)
	case "clearWeeklyStats":
		return clearWeeklyStatsCase(conn)
	case "getLastCleanupTime":
		return getLastCleanupTimeCase(conn)
	case "language_code":
		return languageCodeCase(id, conn, other)
	default:
		return nil, fmt.Errorf("unsupported request: %s", request)
	}
}
