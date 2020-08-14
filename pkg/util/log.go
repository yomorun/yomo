package util

import (
	"fmt"
	"log"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/yomorun/yomo/pkg/env"
)

type LogLevel uint8

const (
	LogLevelNothing LogLevel = iota
	LogLevelError
	LogLevelInfo
	LogLevelDebug
)

const logEnv = "YOMO_LOG_LEVEL"

type Logger interface {
	SetLogLevel(LogLevel)
	SetLogTimeFormat(format string)
	WithPrefix(prefix string) Logger
	Debug() bool

	Errorf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Debugf(format string, args ...interface{})
}

var DefaultLogger Logger

type defaultLogger struct {
	prefix string

	logLevel   LogLevel
	timeFormat string
}

var _ Logger = &defaultLogger{}

func (l *defaultLogger) SetLogLevel(level LogLevel) {
	l.logLevel = level
}

func (l *defaultLogger) SetLogTimeFormat(format string) {
	log.SetFlags(0)
	l.timeFormat = format
}

func (l *defaultLogger) Debugf(format string, args ...interface{}) {
	if l.logLevel == LogLevelDebug {
		l.logMessage(format, args...)
	}
}

func (l *defaultLogger) Infof(format string, args ...interface{}) {
	if l.logLevel >= LogLevelInfo {
		l.logMessage(format, args...)
	}
}

func (l *defaultLogger) Errorf(format string, args ...interface{}) {
	if l.logLevel >= LogLevelError {
		l.logMessage(format, args...)
	}
}

func (l *defaultLogger) logMessage(format string, args ...interface{}) {
	var pre string

	if len(l.timeFormat) > 0 {
		pre = time.Now().Format(l.timeFormat) + " "
	}
	if len(l.prefix) > 0 {
		pre += l.prefix + " "
	}
	log.Printf(pre+format, args...)
}

func (l *defaultLogger) WithPrefix(prefix string) Logger {
	if len(l.prefix) > 0 {
		prefix = l.prefix + " " + prefix
	}
	return &defaultLogger{
		logLevel:   l.logLevel,
		timeFormat: l.timeFormat,
		prefix:     prefix,
	}
}

func (l *defaultLogger) Debug() bool {
	return l.logLevel == LogLevelDebug
}

func init() {
	loadDefaultLogger()
}

var mux sync.Mutex

func loadDefaultLogger() {
	mux.Lock()
	defer mux.Unlock()
	if DefaultLogger == nil {
		DefaultLogger = &defaultLogger{}
		DefaultLogger.SetLogLevel(readLoggingEnv())
	}
}

func readLoggingEnv() LogLevel {
	lvl := strings.ToLower(env.GetString(logEnv, "info"))
	switch lvl {
	case "":
		return LogLevelNothing
	case "debug":
		return LogLevelDebug
	case "info":
		return LogLevelInfo
	case "error":
		return LogLevelError
	default:
		fmt.Fprintln(os.Stderr, "invalid log level")
		return LogLevelNothing
	}
}

func GetLogger(prefix string) Logger {
	if DefaultLogger == nil {
		loadDefaultLogger()
	}
	return Logger.WithPrefix(DefaultLogger, prefix)
}

func GetLoggerOf(obj interface{}) Logger {
	if DefaultLogger == nil {
		loadDefaultLogger()
	}
	return Logger.WithPrefix(DefaultLogger, reflect.TypeOf(obj).Name())
}
