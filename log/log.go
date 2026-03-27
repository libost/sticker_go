package log

import (
	"fmt"
	"log"
	"os"

	"github.com/libost/sticker_go/config"
	C "github.com/libost/sticker_go/constants"
)

func Log(message string, level C.LogLevel) {
	cf, err := config.Init()
	if err != nil {
		log.Printf("Failed to initialize config for logging: %v\n", err)
		return
	}
	var systemdLevel string
	switch cf.Log.Level {
	case "DEBUG":
		systemdLevel = "<7>"
		if level.Number < C.LogLevelDebug.Number {
			return // 当前日志级别不输出
		}
	case "INFO":
		systemdLevel = "<6>"
		if level.Number < C.LogLevelInfo.Number {
			return
		}
	case "WARN":
		systemdLevel = "<4>"
		if level.Number < C.LogLevelWarn.Number {
			return
		}
	case "ERROR":
		systemdLevel = "<3>"
		if level.Number < C.LogLevelError.Number {
			return
		}
	case "FATAL":
		systemdLevel = "<2>"
		if level.Number < C.LogLevelFatal.Number {
			return
		}
	}
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
	_, exists := os.LookupEnv("INVOCATION_ID")
	logMessage := fmt.Sprintf("[%s] [%s] %s%s\n", timestamp, level.Level, message, incorrectTimeNotice)
	if exists {
		logMessage = fmt.Sprintf("%s [%s] %s%s\n", systemdLevel, level.Level, message, incorrectTimeNotice)
	}
	logToFile(message, level)
	os.Stdout.WriteString(logMessage)
	if level == C.LogLevelFatal {
		os.Exit(1)
	}
}

func logToFile(message string, level C.LogLevel) {
	timestamp, isTimeRight := timeNow()
	var incorrectTimeNotice string
	if !isTimeRight {
		incorrectTimeNotice = " (Current time may be incorrect due to timezone issues, check config)"
	}
	logFilePath := fmt.Sprintf("%s/log_%s.log", C.LogDir, timestamp[:10]) // 每天一个日志文件，格式为 log_YYYY-MM-DD.log
	logMessage := fmt.Sprintf("[%s] [%s] %s%s\n", timestamp, level.Level, message, incorrectTimeNotice)
	f, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Failed to open log file: %v\n", err)
		return
	}
	defer f.Close()
	if _, err := f.WriteString(logMessage); err != nil {
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
