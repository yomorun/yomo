package logger

import (
	"os"
	"strings"

	"github.com/yomorun/yomo/core/log"
)

var logger = Default(isEnableDebug())

// EnableDebug enables the development model for logging.
func EnableDebug() {
	logger = Default(true)
}

// Printf prints a formated message without a specified level.
func Printf(format string, v ...interface{}) {
	logger.Printf(format, v...)
}

// Debugf logs a message at DebugLevel.
func Debugf(template string, args ...interface{}) {
	logger.Debugf(template, args...)
}

// Infof logs a message at InfoLevel.
func Infof(template string, args ...interface{}) {
	logger.Infof(template, args...)
}

// Warnf logs a message at WarnLevel.
func Warnf(template string, args ...interface{}) {
	logger.Warnf(template, args...)
}

// Errorf logs a message at ErrorLevel.
func Errorf(template string, args ...interface{}) {
	logger.Errorf(template, args...)
}

// isEnableDebug indicates whether the debug is enabled.
func isEnableDebug() bool {
	return os.Getenv("YOMO_ENABLE_DEBUG") == "true"
}

// isJSONFormat indicates whether the log is in JSON format.
func isJSONFormat() bool {
	return os.Getenv("YOMO_LOG_FORMAT") == "json"
}

func logFormat() string {
	return os.Getenv("YOMO_LOG_FORMAT")
}

func logLevel() log.Level {
	envLevel := strings.ToLower(os.Getenv("YOMO_LOG_LEVEL"))
	level := log.ErrorLevel
	switch envLevel {
	case "debug":
		return log.DebugLevel
	case "info":
		return log.InfoLevel
	case "warn":
		return log.WarnLevel
	case "error":
		return log.ErrorLevel
	}
	return level
}

func output() string {
	return strings.ToLower(os.Getenv("YOMO_LOG_OUTPUT"))
}

func errorOutput() string {
	return strings.ToLower(os.Getenv("YOMO_LOG_ERROR_OUTPUT"))
}
