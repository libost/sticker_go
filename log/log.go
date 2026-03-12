package log

import (
	"fmt"
	"log"
	"os"

	"libost/sticker_go/config"
	C "libost/sticker_go/constants"
)

func Log(message string, level string) {
	logDir := C.LogDir
	logInfo, err := os.Stat(logDir) // 检查日志目录是否存在
	if os.IsNotExist(err) || (err == nil && !logInfo.IsDir()) {
		err = os.Mkdir(logDir, 0755)
		if err != nil {
			log.Fatal("failed to create log directory")
		}
	}
	timestamp, isTimeRight := timeNow()
	var incorrectTimeNotice string
	if !isTimeRight {
		incorrectTimeNotice = " (Current time may be incorrect due to timezone issues, check config)"
	}
	logMessage := fmt.Sprintf("[%s] [%s] %s%s\n", timestamp, level, message, incorrectTimeNotice)
	logToFile(logMessage)
	os.Stdout.WriteString(logMessage)
}

func logToFile(message string) {
	timestamp, _ := timeNow()
	logFilePath := fmt.Sprintf("%s/log_%s.log", C.LogDir, timestamp[:10]) // 每天一个日志文件，格式为 log_YYYY-MM-DD.log
	f, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Failed to open log file: %v\n", err)
		return
	}
	defer f.Close()
	if _, err := f.WriteString(message); err != nil {
		fmt.Printf("Failed to write to log file: %v\n", err)
	}
}

func timeNow() (string, bool) {
	cf, err := config.Init()
	if err != nil {
		log.Fatal("failed to initialize config")
	}
	timestamp, err := C.CurrentTime(cf.Misc.Timezone)
	var isTimeRight bool
	if err != nil {
		isTimeRight = false
	} else {
		isTimeRight = true
	}
	return timestamp, isTimeRight
}
