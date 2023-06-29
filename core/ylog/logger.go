// Package ylog provides a slog.Logger instance for logging.
// ylog also provides a default slog.Logger, the default logger is build from environment.
//
// ylog allows to call log api directly, like:
//
//	ylog.Debug("test", "name", "yomo")
//	ylog.Info("test", "name", "yomo")
//	ylog.Warn("test", "name", "yomo")
//	ylog.Error("test", "name", "yomo")
package ylog

import (
	"io"
	"log"
	"os"
	"strings"

	"github.com/caarlos0/env/v6"
	"golang.org/x/exp/slog"
	"gopkg.in/natefinch/lumberjack.v2"
)

var defaultLogger = Default()

// SetDefault set global logger.
func SetDefault(logger *slog.Logger) { defaultLogger = logger }

// Debug logs a message at debug level.
func Debug(msg string, keyvals ...interface{}) {
	defaultLogger.Debug(msg, keyvals...)
}

// Info logs a message at info level.
func Info(msg string, keyvals ...interface{}) {
	defaultLogger.Info(msg, keyvals...)
}

// Warn logs a message at warn level.
func Warn(msg string, keyvals ...interface{}) {
	defaultLogger.Warn(msg, keyvals...)
}

// Error logs a message at error level.
func Error(msg string, keyvals ...interface{}) {
	defaultLogger.Error(msg, keyvals...)

}

// Config is the config of slog, the config is from environment.
type Config struct {
	// Verbose indicates if logger log code line, use false for production.
	Verbose bool `env:"YOMO_LOG_VERBOSE" envDefault:"false"`

	// Level can be one of `debug`, `info`, `warn`, `error`
	Level string `env:"YOMO_LOG_LEVEL" envDefault:"info"`

	// Output is the filename of log file,
	// The default is stdout.
	Output string `env:"YOMO_LOG_OUTPUT"`

	// ErrorOutput is the filename of errlog file,
	// The default is stderr.
	ErrorOutput string `env:"YOMO_LOG_ERROR_OUTPUT"`

	// Format supports text and json,
	// The default is text.
	Format string `env:"YOMO_LOG_FORMAT" envDefault:"text"`

	// MaxSize is the maximum size in megabytes of the log file before it gets rotated.
	// It defaults to 100 megabytes.
	MaxSize int `env:"YOMO_LOG_MAX_SIZE"`

	// MaxBackups is the maximum number of old log files to retain.
	// The default is to retain all old log files (though MaxAge may still cause them to get deleted.)
	MaxBackups int `env:"YOMO_LOG_MAX_BACKUPS"`

	// MaxAge is the maximum number of days to retain old log files based on the timestamp encoded in their filename.
	// Note that a day is defined as 24 hours and may not exactly correspond to calendar days due to daylight savings, leap seconds, etc.
	// The default is not to remove old log files based on age.
	MaxAge int `env:"YOMO_LOG_MAX_AGE"`

	// LocalTime determines if the time used for formatting the timestamps in backup files is the computer's local time.
	// The default is to use UTC time.
	LocalTime bool `env:"YOMO_LOG_LOCAL_TIME"`

	// Compress determines if the rotated log files should be compressed using gzip.
	// The default is not to perform compression.
	Compress bool `env:"YOMO_LOG_COMPRESS"`

	// DisableTime disable time key, It's a pravited field, Just for testing.
	DisableTime bool
}

// Default returns a slog.Logger according to enviroment.
func Default() *slog.Logger {
	var conf Config
	if err := env.Parse(&conf); err != nil {
		log.Fatalf("%+v\n", err)
	}
	return NewFromConfig(conf)
}

// NewFromConfig returns a slog.Logger according to conf.
func NewFromConfig(conf Config) *slog.Logger {
	return slog.New(NewHandlerFromConfig(conf))
}

func parseToWriter(conf Config, path string, defaultWriter io.Writer) io.Writer {
	switch strings.ToLower(path) {
	case "stdout":
		return os.Stdout
	case "stderr":
		return os.Stderr
	default:
		if path != "" {
			return &lumberjack.Logger{
				Filename:   path,
				MaxSize:    conf.MaxSize,
				MaxAge:     conf.MaxAge,
				MaxBackups: conf.MaxBackups,
				LocalTime:  conf.LocalTime,
				Compress:   conf.Compress,
			}
		}
		return defaultWriter
	}
}

func parseToSlogLevel(stringLevel string) slog.Level {
	level := slog.LevelDebug
	switch strings.ToLower(stringLevel) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	return level
}
