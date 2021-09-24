package logger

import (
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
	// Debugf logs a message at DebugLevel.
	Debugf(template string, args ...interface{})

	// Info logs a message at InfoLevel.
	Info(msg string, kvPairs ...interface{})
	// Infof logs a message at InfoLevel.
	Infof(template string, args ...interface{})

	// Warn logs a message at WarnLevel.
	Warn(msg string, kvPairs ...interface{})
	// Warnf logs a message at WarnLevel.
	Warnf(template string, args ...interface{})

	// Error logs a message at ErrorLevel.
	Error(msg string, kvPairs ...interface{})
	// Errorf logs a message at ErrorLevel.
	Errorf(template string, args ...interface{})

	// Panic logs a message at PanicLevel.
	Panic(msg string, kvPairs ...interface{})
	// Panicf logs a message at PanicLevel.
	Panicf(template string, args ...interface{})

	// Fatal logs a message at FatalLevel.
	Fatal(msg string, kvPairs ...interface{})
	// Fatalf logs a message at FatalLevel.
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

// Debugf logs a message at DebugLevel.
func Debugf(template string, args ...interface{}) {
	logger.Debugf(template, args...)
}

// Info logs a message at InfoLevel.
func Info(msg string, kvPairs ...interface{}) {
	logger.Info(msg, kvPairs...)
}

// Infof logs a message at InfoLevel.
func Infof(template string, args ...interface{}) {
	logger.Infof(template, args...)
}

// Warn logs a message at WarnLevel.
func Warn(msg string, kvPairs ...interface{}) {
	logger.Warn(msg, kvPairs...)
}

// Warnf logs a message at WarnLevel.
func Warnf(template string, args ...interface{}) {
	logger.Warnf(template, args...)
}

// Error logs a message at ErrorLevel.
func Error(msg string, kvPairs ...interface{}) {
	logger.Error(msg, kvPairs...)
}

// Errorf logs a message at ErrorLevel.
func Errorf(template string, args ...interface{}) {
	logger.Errorf(template, args...)
}

// Panic logs a message at PanicLevel.
func Panic(msg string, kvPairs ...interface{}) {
	logger.Panic(msg, kvPairs...)
}

// Panicf logs a message at PanicLevel.
func Panicf(template string, args ...interface{}) {
	logger.Panicf(template, args...)
}

// Fatal logs a message at FatalLevel.
func Fatal(msg string, kvPairs ...interface{}) {
	logger.Fatal(msg, kvPairs...)
}

// Fatalf logs a message at FatalLevel.
func Fatalf(template string, args ...interface{}) {
	logger.Fatalf(template, args...)
}

// isEnableDebug indicates whether the debug is enabled.
func isEnableDebug() bool {
	return os.Getenv("YOMO_ENABLE_DEBUG") == "true"
}

// isJSONFormat indicates whether the log is in JSON format.
func isJSONFormat() bool {
	return os.Getenv("YOMO_LOG_FORMAT") == "json"
}

func logLevel() string {
	return strings.ToLower(os.Getenv("YOMO_LOG_LEVEL"))
}
