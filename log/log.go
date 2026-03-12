package log

import (
	"fmt"
	"log"
	"os"
	"time"

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
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logMessage := fmt.Sprintf("[%s] [%s] %s\n", timestamp, level, message)
	logToFile(message, level)
	os.Stdout.WriteString(logMessage)
}

func logToFile(message string, level string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logMessage := fmt.Sprintf("[%s] [%s] %s\n", timestamp, level, message)
	logFilePath := fmt.Sprintf("%s/log_%s.log", C.LogDir, time.Now().Format("2006-01-02"))
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
