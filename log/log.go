package log

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	"runtime/metrics"
	"runtime/pprof"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"
	"github.com/libost/sticker_go/config"
	C "github.com/libost/sticker_go/constants"
)

func Log(message string, level C.LogLevel) {
	cf := config.AppConfig
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
	preMessage := fmt.Sprintf("[%s] %s%s\n", level.Level, message, incorrectTimeNotice)
	logMessage := fmt.Sprintf("[%s] %s", timestamp, preMessage)
	_, exists := os.LookupEnv("INVOCATION_ID")
	if exists {
		logMessage = fmt.Sprintf("%s %s", systemdLevel, preMessage)
	}
	logToFile(preMessage)
	os.Stdout.WriteString(logMessage)
	if level == C.LogLevelFatal {
		dumpFile, err := os.Create(fmt.Sprintf("pprof_%d.pprof", time.Now().Unix()))
		if err != nil {
			log.Printf("Failed to create pprof dump file: %v", err)
		}
		defer dumpFile.Close()
		pprof.WriteHeapProfile(dumpFile)
		log.Printf("Software Crashed. Heap profile written to %s. Check working directory for the file.", dumpFile.Name())
		os.Exit(1)
	}
}

func logToFile(message string) {
	timestamp, _ := timeNow()
	logFilePath := fmt.Sprintf("%s/log_%s.log", C.LogDir, timestamp[:10]) // 每天一个日志文件，格式为 log_YYYY-MM-DD.log
	logMessage := fmt.Sprintf("[%s] %s", timestamp, message)
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
	cf := config.AppConfig
	timestamp, err := C.CurrentTime(cf.Misc.Timezone)
	var isTimeRight bool
	if err != nil {
		isTimeRight = false
	} else {
		isTimeRight = true
	}
	return timestamp, isTimeRight
}

func memoryUsage() (uint64, error) {
	samples := make([]metrics.Sample, 1)
	samples[0].Name = "/memory/classes/heap/objects:bytes"
	metrics.Read(samples)
	var memoryBytes uint64
	_, exists := os.LookupEnv("INVOCATION_ID")
	if exists {
		ctx := context.Background()
		conn, err := dbus.NewSystemConnectionContext(ctx)
		if err != nil {
			Log(fmt.Sprintf("Failed to connect to systemd dbus: %v", err), C.LogLevelFatal)
		}
		defer conn.Close()
		prop, err := conn.GetServicePropertyContext(ctx, "sticker_go.service", "MemoryCurrent")
		if err != nil {
			Log(fmt.Sprintf("Failed to get memory usage from systemd cgroup: %v", err), C.LogLevelError)
			memoryBytes = 18446744073709551615 // 2^64-1, 表示未知内存使用量
		}
		memoryBytes = prop.Value.Value().(uint64)
	} else {
		switch samples[0].Value.Kind() {
		case metrics.KindUint64:
			memoryBytes = samples[0].Value.Uint64()
		case metrics.KindFloat64:
			memoryBytes = uint64(samples[0].Value.Float64())
		default:
			var mem runtime.MemStats
			runtime.ReadMemStats(&mem)
			memoryBytes = mem.Alloc
		}
	}
	return memoryBytes, nil
}

func memLogtoFile(memoryBytes uint64) error {
	memFilePath := fmt.Sprintf("%s/memory_%s.log", C.LogDir, time.Now().Format("2006-01-02"))
	memMessage := fmt.Sprintf("[%s] Memory Usage: %d bytes\n", time.Now().Format(time.RFC3339), memoryBytes)
	f, err := os.OpenFile(memFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Failed to open memory log file: %v\n", err)
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(memMessage); err != nil {
		fmt.Printf("Failed to write to memory log file: %v\n", err)
		return err
	}
	return nil
}

func MemoryLog() error {
	if config.AppConfig.Log.Level != "DEBUG" {
		return nil
	}
	go func() {
		memoryBytes, err := memoryUsage()
		if err != nil {
			Log(fmt.Sprintf("Failed to get memory usage: %v", err), C.LogLevelError)
			return
		}
		err = memLogtoFile(memoryBytes)
		if err != nil {
			Log(fmt.Sprintf("Failed to log memory usage: %v", err), C.LogLevelError)
			return
		}
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			memoryBytes, err := memoryUsage()
			if err != nil {
				Log(fmt.Sprintf("Failed to get memory usage: %v", err), C.LogLevelError)
				continue
			}
			err = memLogtoFile(memoryBytes)
			if err != nil {
				Log(fmt.Sprintf("Failed to log memory usage: %v", err), C.LogLevelError)
			}
		}
	}()
	return nil
}
