package database

import (
	"database/sql"
	"embed"
	"fmt"
	"sync"
	"time"

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
		db, dbErr = sql.Open("sqlite", "./sticker_go.db")
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

func userExists(conn *sql.DB, id int64) (bool, error) {
	var count int
	err := conn.QueryRow("SELECT COUNT(1) FROM USERPOOL WHERE user_id = ?", id).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
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

func Init(request string, id int64, other map[string]any) (map[string]any, error) {
	conn, err := getDB()
	if err != nil {
		return nil, err
	}

	data := map[string]any{"user_id": id, "exists": false}

	if id > 0 {
		exists, err := userExists(conn, id)
		if err != nil {
			return nil, err
		}
		data["exists"] = exists
	}

	switch request {
	case "init":
		return data, nil
	case "create":
		if id <= 0 {
			return data, nil
		}
		if err := createUserIfNotExists(conn, id); err != nil {
			return nil, err
		}
		data["exists"] = true
		return data, nil
	case "usage":
		if id <= 0 {
			return data, nil
		}
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
	case "usageRecord":
		if id <= 0 {
			return data, nil
		}
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
		data["exists"] = true
		return data, nil
	case "user_group":
		if id <= 0 {
			return data, nil
		}
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
	case "stats":
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
		return data, nil
	case "set_group":
		if id <= 0 {
			return data, nil
		}
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
	case "reset_usage":
		if id <= 0 {
			return data, nil
		}
		if _, err := conn.Exec("UPDATE USERPOOL SET usage_count = 0, last_cycle_starts_at = unixepoch() WHERE user_id = ?", id); err != nil {
			return nil, err
		}
		data["exists"] = true
		data["usage"] = float64(0)
		data["last_cycle_starts_at"] = float64(time.Now().Unix())
		return data, nil
	default:
		return nil, fmt.Errorf("unsupported request: %s", request)
	}
}
