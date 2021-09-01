package logger

import (
	"fmt"
	"os"
	"strings"
)

// Logger is the interface for logger.
type Logger interface {
	// Print prints a farmat message without a specified level.
	Print(v ...interface{})

	// Printf prints a formated message without a specified level.
	Printf(format string, v ...interface{})

	// Debug logs a message at DebugLevel.
	Debug(msg string, kvPairs ...interface{})
	Debugf(template string, args ...interface{})

	// Info logs a message at InfoLevel.
	Info(msg string, kvPairs ...interface{})
	Infof(template string, args ...interface{})

	// Warn logs a message at WarnLevel.
	Warn(msg string, kvPairs ...interface{})
	Warnf(template string, args ...interface{})

	// Error logs a message at ErrorLevel.
	Error(msg string, kvPairs ...interface{})
	Errorf(template string, args ...interface{})

	// Panic logs a message at PanicLevel.
	Panic(msg string, kvPairs ...interface{})
	Panicf(template string, args ...interface{})

	// Fatal logs a message at FatalLevel.
	// The logger then calls os.Exit(1).
	Fatal(msg string, kvPairs ...interface{})
	Fatalf(template string, args ...interface{})
}

var logger = newLogger(isEnableDebug())

// EnableDebug enables the development model for logging.
func EnableDebug() {
	logger = newLogger(true)
}

// Print prints a farmat message without a specified level.
func Print(v ...interface{}) {
	logger.Print(v...)
}

// Printf prints a formated message without a specified level.
func Printf(format string, v ...interface{}) {
	logger.Printf(format, v...)
}

// Debug logs a message at DebugLevel.
func Debug(msg string, kvPairs ...interface{}) {
	logger.Debug(msg, kvPairs...)
}

func Debugf(template string, args ...interface{}) {
	logger.Debugf(template, args...)
}

// Info logs a message at InfoLevel.
func Info(msg string, kvPairs ...interface{}) {
	logger.Info(msg, kvPairs...)
}

func Infof(template string, args ...interface{}) {
	logger.Infof(template, args...)
}

// Warn logs a message at WarnLevel.
func Warn(msg string, kvPairs ...interface{}) {
	logger.Warn(msg, kvPairs...)
}

func Warnf(template string, args ...interface{}) {
	logger.Warnf(template, args...)
}

// Error logs a message at ErrorLevel.
func Error(msg string, kvPairs ...interface{}) {
	logger.Error(msg, kvPairs...)
}

func Errorf(template string, args ...interface{}) {
	logger.Errorf(template, args...)
}

// Panic logs a message at PanicLevel.
func Panic(msg string, kvPairs ...interface{}) {
	logger.Panic(msg, kvPairs...)
}

func Panicf(template string, args ...interface{}) {
	logger.Panicf(template, args...)
}

// Fatal logs a message at FatalLevel.
// The logger then calls os.Exit(1).
func Fatal(msg string, kvPairs ...interface{}) {
	logger.Fatal(msg, kvPairs...)
}

func Fatalf(template string, args ...interface{}) {
	logger.Fatalf(template, args...)
}

// BytesString formats the bytes to string.
func BytesString(bytes []byte) string {
	return fmt.Sprintf("%v", bytes)
}

// isEnableDebug indicates whether the debug is enabled.
func isEnableDebug() bool {
	if os.Getenv("YOMO_ENABLE_DEBUG") == "true" {
		return true
	}
	return false
}

// isJSONFormat indicates whether the log is in JSON format.
func isJSONFormat() bool {
	if os.Getenv("YOMO_LOG_FORMAT") == "json" {
		return true
	}
	return false
}

func logLevel() string {
	return strings.ToLower(os.Getenv("YOMO_LOG_LEVEL"))
}
