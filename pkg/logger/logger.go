package logger

import (
	"fmt"
	"os"
)

// Logger
type Logger interface {
	// Print prints a farmat message without a specified level.
	Print(v ...interface{})

	// Printf prints a formated message without a specified level.
	Printf(format string, v ...interface{})

	// Debug logs a message at DebugLevel.
	Debug(msg string, fields ...interface{})

	// Info logs a message at InfoLevel.
	Info(msg string, fields ...interface{})

	// Warn logs a message at WarnLevel.
	Warn(msg string, fields ...interface{})

	// Error logs a message at ErrorLevel.
	Error(msg string, fields ...interface{})

	// Panic logs a message at PanicLevel.
	Panic(msg string, fields ...interface{})

	// Fatal logs a message at FatalLevel.
	// The logger then calls os.Exit(1).
	Fatal(msg string, fields ...interface{})
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
func Debug(msg string, fields ...interface{}) {
	logger.Debug(msg, fields...)
}

// Info logs a message at InfoLevel.
func Info(msg string, fields ...interface{}) {
	logger.Info(msg, fields...)
}

// Warn logs a message at WarnLevel.
func Warn(msg string, fields ...interface{}) {
	logger.Warn(msg, fields...)
}

// Error logs a message at ErrorLevel.
func Error(msg string, fields ...interface{}) {
	logger.Error(msg, fields...)
}

// Panic logs a message at PanicLevel.
func Panic(msg string, fields ...interface{}) {
	logger.Panic(msg, fields...)
}

// Fatal logs a message at FatalLevel.
// The logger then calls os.Exit(1).
func Fatal(msg string, fields ...interface{}) {
	logger.Fatal(msg, fields...)
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

// isProductionMode indicates whether the log is in production model.
func isJsonFormat() bool {
	if os.Getenv("YOMO_LOG_FORMAT") == "json" {
		return true
	}
	return false
}
