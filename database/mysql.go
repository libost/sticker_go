//go:build mysql

package database

import (
	"database/sql"
	"embed"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/libost/sticker_go/config"
	C "github.com/libost/sticker_go/constants"

	_ "github.com/go-sql-driver/mysql"
)

//go:embed schema_mysql.sql
var schemaFS embed.FS

var (
	db     *sql.DB
	dbOnce sync.Once
	dbErr  error
)

func mysqlDSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=true&loc=Local",
		config.AppConfig.MySQL.Username,
		config.AppConfig.MySQL.Password,
		config.AppConfig.MySQL.Host,
		config.AppConfig.MySQL.Port,
		"sticker_go",
	)
}

func applySchema(conn *sql.DB) error {
	schema, err := schemaFS.ReadFile("schema_mysql.sql")
	if err != nil {
		return err
	}
	for stmt := range strings.SplitSeq(string(schema), ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := conn.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func getDB() (*sql.DB, error) {
	dbOnce.Do(func() {
		db, dbErr = sql.Open("mysql", mysqlDSN())
		if dbErr != nil {
			return
		}
		db.SetMaxIdleConns(5)
		db.SetMaxOpenConns(10)
		if dbErr = db.Ping(); dbErr != nil {
			return
		}
		dbErr = applySchema(db)
	})

	if dbErr != nil {
		return nil, dbErr
	}
	return db, nil
}

func ensureLanguageCodeColumn(conn *sql.DB) error {
	var count int64
	err := conn.QueryRow(`
		SELECT COUNT(1)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = 'USERPOOL'
		  AND COLUMN_NAME = 'language_code'
	`).Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		_, err = conn.Exec("ALTER TABLE USERPOOL ADD COLUMN language_code VARCHAR(32) NULL")
		return err
	}
	return nil
}

func createUserIfNotExists(conn *sql.DB, id int64) error {
	_, err := conn.Exec(
		"INSERT IGNORE INTO USERPOOL (user_id, obfu_id, created_at, last_cycle_starts_at) VALUES (?, ?, ?, ?)",
		id,
		fmt.Sprintf("u_%d_%d", id, time.Now().UnixNano()),
		time.Now().Unix(),
		time.Now().Unix(),
	)
	return err
}

func normalizeUsageCycle(conn *sql.DB, id int64) error {
	_, err := conn.Exec(
		`UPDATE USERPOOL
		 SET usage_count = 0,
		     last_cycle_starts_at = UNIX_TIMESTAMP()
		 WHERE user_id = ?
		   AND UNIX_TIMESTAMP() - last_cycle_starts_at >= 30 * 24 * 3600`,
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
	case int32:
		return int(n), nil
	case float64:
		return int(n), nil
	default:
		return 0, fmt.Errorf("invalid usage type")
	}
}

func toAmountInt64(other map[string]any) (int64, error) {
	if other == nil {
		return 0, fmt.Errorf("missing amount")
	}
	v, ok := other["amount"]
	if !ok {
		return 0, fmt.Errorf("missing amount")
	}
	switch n := v.(type) {
	case int:
		return int64(n), nil
	case int64:
		return n, nil
	case int32:
		return int64(n), nil
	case float64:
		return int64(n), nil
	default:
		return 0, fmt.Errorf("invalid amount type")
	}
}

func logIntoDonateLogs(conn *sql.DB, id int64, amount int64, payload string) error {
	_, err := conn.Exec(
		"INSERT INTO DONATION_LOGS (user_id, amount, timestamp, payload, telegram_payment_charge_id, provider_payment_charge_id, status) VALUES (?, ?, ?, ?, ?, ?, 'pending')",
		id,
		amount,
		time.Now().Unix(),
		payload,
		"pending",
		"pending",
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
	if err := ensureLanguageCodeColumn(conn); err != nil {
		return nil, err
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
		"INSERT INTO STATISTICS (weekday, daily_usage_count) VALUES (?, ?) ON DUPLICATE KEY UPDATE daily_usage_count = daily_usage_count + ?",
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
	if _, err := conn.Exec("UPDATE USERPOOL SET usage_count = 0, last_cycle_starts_at = UNIX_TIMESTAMP() WHERE user_id = ?", id); err != nil {
		return nil, err
	}
	data["exists"] = true
	data["usage"] = float64(0)
	data["last_cycle_starts_at"] = float64(time.Now().Unix())
	return data, nil
}

func donateInitCase(id int64, conn *sql.DB, other map[string]any) (map[string]any, error) {
	data := map[string]any{"user_id": id, "exists": true}
	amount, err := toAmountInt64(other)
	if err != nil {
		return nil, err
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
	telegamChargeID, ok := other["telegram_charge_id"].(string)
	if !ok {
		return nil, fmt.Errorf("missing telegram_charge_id")
	}
	_, err := conn.Exec(
		"UPDATE DONATION_LOGS SET status = 'refunded' WHERE telegram_payment_charge_id = ?",
		telegamChargeID,
	)
	if err != nil {
		return data, err
	}
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
	if _, err = tx.Exec("DELETE FROM STATISTICS"); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if _, err = tx.Exec("INSERT INTO LAST_CLEANUP (id, last_cleanup_at) VALUES (1, UNIX_TIMESTAMP()) ON DUPLICATE KEY UPDATE last_cleanup_at = UNIX_TIMESTAMP()"); err != nil {
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
	var lastCleanup sql.NullInt64
	err := conn.QueryRow("SELECT last_cleanup_at FROM LAST_CLEANUP WHERE id = 1").Scan(&lastCleanup)
	if err == sql.ErrNoRows || !lastCleanup.Valid {
		data["last_cleanup_at"] = float64(0)
		return data, nil
	}
	if err != nil {
		return nil, err
	}
	data["last_cleanup_at"] = float64(lastCleanup.Int64)
	return data, nil
}

func languageCodeCase(id int64, conn *sql.DB, other map[string]any) (map[string]any, error) {
	data := map[string]any{"user_id": id, "exists": true}
	requestType, _ := other["type"].(string)
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

func queryUserUsage(id int64, conn *sql.DB) (map[string]any, error) {
	var totalUsage int64
	var usageCount int64
	err := conn.QueryRow("SELECT total_usage_count, usage_count FROM USERPOOL WHERE user_id = ?", id).Scan(&totalUsage, &usageCount)
	if err == sql.ErrNoRows {
		return map[string]any{"user_id": id, "exists": false}, nil
	}
	if err != nil {
		return nil, err
	}
	data := map[string]any{
		"user_id":     id,
		"exists":      true,
		"total_usage": totalUsage,
		"usage":       usageCount,
	}
	return data, nil
}

func genGraceKey(id int64, conn *sql.DB) (map[string]any, error) {
	key := uuid.New().String()
	_, err := conn.Exec(
		"INSERT INTO GRACE_KEY (operator, uuid, generated_at, expires_at) VALUES (?, ?, ?, ?)",
		id,
		key,
		time.Now().Unix(),
		time.Now().Unix()+3600,
	)
	return map[string]any{"grace_key": key}, err
}

func useGraceKey(id int64, conn *sql.DB, other map[string]any) (map[string]any, error) {
	graceKey, ok := other["grace_key"].(string)
	if !ok {
		return nil, fmt.Errorf("missing grace_key")
	}
	var expiredAt int64
	err := conn.QueryRow("SELECT expires_at FROM GRACE_KEY WHERE uuid = ?", graceKey).Scan(&expiredAt)
	if err == sql.ErrNoRows {
		return nil, C.ErrInvalidGraceKey
	}
	if err != nil {
		return nil, err
	}
	if time.Now().Unix() > expiredAt {
		return nil, C.ErrGraceKeyExpired
	}
	if _, err = conn.Exec("UPDATE USERPOOL SET usage_count = 0, last_cycle_starts_at = UNIX_TIMESTAMP() WHERE user_id = ?", id); err != nil {
		return nil, err
	}
	if _, err = conn.Exec("DELETE FROM GRACE_KEY WHERE uuid = ?", graceKey); err != nil {
		return nil, err
	}
	return nil, nil
}

func getPersistentData(conn *sql.DB) (map[string]any, error) {
	var lastApiEndpoint sql.NullString
	var lastApiToken sql.NullString
	err := conn.QueryRow("SELECT last_api_endpoint, last_api_token FROM PERSISTENT_DATA WHERE id = 1").Scan(&lastApiEndpoint, &lastApiToken)
	if err == sql.ErrNoRows {
		return map[string]any{"last_api_endpoint": "", "last_api_token": ""}, nil
	}
	if err != nil {
		return nil, err
	}
	if !lastApiEndpoint.Valid {
		lastApiEndpoint.String = ""
	}
	if !lastApiToken.Valid {
		lastApiToken.String = ""
	}
	return map[string]any{"last_api_endpoint": lastApiEndpoint.String, "last_api_token": lastApiToken.String}, nil
}

func writePersistentData(conn *sql.DB, other map[string]any) (map[string]any, error) {
	lastApiEndpoint, _ := other["last_api_endpoint"].(string)
	lastApiToken, ok := other["last_api_token"].(string)
	if !ok {
		return nil, fmt.Errorf("missing last_api_token")
	}
	_, err := conn.Exec(
		"INSERT INTO PERSISTENT_DATA (id, last_api_endpoint, last_api_token) VALUES (1, ?, ?) ON DUPLICATE KEY UPDATE last_api_endpoint = VALUES(last_api_endpoint), last_api_token = VALUES(last_api_token)",
		lastApiEndpoint,
		lastApiToken,
	)
	if err != nil {
		return nil, err
	}
	return map[string]any{"last_api_endpoint": lastApiEndpoint, "last_api_token": lastApiToken}, nil
}

func refreshUsageCounterCase(conn *sql.DB) (map[string]any, error) {
	_, err := conn.Exec("UPDATE USERPOOL SET usage_count = 0, last_cycle_starts_at = UNIX_TIMESTAMP() WHERE UNIX_TIMESTAMP() - last_cycle_starts_at >= 24 * 3600")
	if err != nil {
		return nil, err
	}
	return map[string]any{}, nil
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
	case "queryUserUsage":
		return queryUserUsage(id, conn)
	case "genGraceKey":
		return genGraceKey(id, conn)
	case "useGraceKey":
		return useGraceKey(id, conn, other)
	case "getPersistentData":
		return getPersistentData(conn)
	case "writePersistentData":
		return writePersistentData(conn, other)
	case "refreshUsageCounter":
		return refreshUsageCounterCase(conn)
	default:
		return nil, fmt.Errorf("unsupported request: %s", request)
	}
}
