package logger

import (
	"fmt"
	"os"
)

// Logger is the interface for logging
type Logger interface {
	// Print prints a farmat message without a specified level.
	Print(v ...interface{})

	// Printf prints a formated message without a specified level.
	Printf(format string, v ...interface{})

	// Debug logs a message at DebugLevel.
	Debug(msg string, kvPairs ...interface{})

	// Info logs a message at InfoLevel.
	Info(msg string, kvPairs ...interface{})

	// Warn logs a message at WarnLevel.
	Warn(msg string, kvPairs ...interface{})

	// Error logs a message at ErrorLevel.
	Error(msg string, kvPairs ...interface{})

	// Panic logs a message at PanicLevel.
	Panic(msg string, kvPairs ...interface{})

	// Fatal logs a message at FatalLevel.
	// The logger then calls os.Exit(1).
	Fatal(msg string, kvPairs ...interface{})
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

// Info logs a message at InfoLevel.
func Info(msg string, kvPairs ...interface{}) {
	logger.Info(msg, kvPairs...)
}

// Warn logs a message at WarnLevel.
func Warn(msg string, kvPairs ...interface{}) {
	logger.Warn(msg, kvPairs...)
}

// Error logs a message at ErrorLevel.
func Error(msg string, kvPairs ...interface{}) {
	logger.Error(msg, kvPairs...)
}

// Panic logs a message at PanicLevel.
func Panic(msg string, kvPairs ...interface{}) {
	logger.Panic(msg, kvPairs...)
}

// Fatal logs a message at FatalLevel.
// The logger then calls os.Exit(1).
func Fatal(msg string, kvPairs ...interface{}) {
	logger.Fatal(msg, kvPairs...)
}

// BytesString formats the bytes to string.
func BytesString(bytes []byte) string {
	return fmt.Sprintf("%v", bytes)
}

// isEnableDebug indicates whether the debug is enabled.
func isEnableDebug() bool {
	return os.Getenv("YOMO_ENABLE_DEBUG") == "true"
}

// isJSONFormat indicates whether the log is in JSON format.
func isJSONFormat() bool {
	return os.Getenv("YOMO_LOG_FORMAT") == "json"
}
