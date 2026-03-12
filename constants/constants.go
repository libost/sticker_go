package constants

import "time"

const (
	CacheDir   = "./cache/"
	ConfigFile = "./config.yaml"
	LogDir     = "./logs/"
)

const (
	LogLevelDebug = "DEBUG"
	LogLevelInfo  = "INFO"
	LogLevelWarn  = "WARN"
	LogLevelError = "ERROR"
	LogLevelFatal = "FATAL"
)

func CurrentTime(timezone string) (string, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.FixedZone("UTC", 0) // Fallback to UTC if timezone is invalid
	}
	return time.Now().In(loc).Format("2006-01-02 15:04:05"), err
}
