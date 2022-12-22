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
	"strconv"
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
func Error(msg string, err error, keyvals ...interface{}) {
	defaultLogger.Error(msg, err, keyvals...)
}

// Config is the config of slog, the config is from environment.
type Config struct {
	// Verbose indicates if logger log code line, use false for production.
	Verbose bool `env:"YOMO_LOG_VERBOSE" envDefault:"false"`

	// the log level, It can be one of `debug`, `info`, `warn`, `error`
	Level string `env:"YOMO_LOG_LEVEL" envDefault:"info"`

	// log output file path, It's stdout if not set.
	Output string `env:"YOMO_LOG_OUTPUT"`

	// error log output file path, It's stderr if not set.
	ErrorOutput string `env:"YOMO_LOG_ERROR_OUTPUT"`

	// log format, support text and json.
	Format string `env:"YOMO_LOG_FORMAT" envDefault:"text"`

	// DisableTime disable time key, It's a pravited field, Just for testing.
	DisableTime bool
}

// DebugFrameSize is use for log dataFrame,
// It means that only logs the first DebugFrameSize bytes if the data is large than DebugFrameSize bytes.
//
// DebugFrameSize is default to 16,
// if env `YOMO_DEBUG_FRAME_SIZE` is setted and It's an int number, Set the env value to DebugFrameSize.
var DebugFrameSize = 16

func init() {
	if e := os.Getenv("YOMO_DEBUG_FRAME_SIZE"); e != "" {
		if val, err := strconv.Atoi(e); err == nil {
			DebugFrameSize = val
		}
	}
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

func parseToWriter(path string, defaultWriter io.Writer) (io.Writer, error) {
	switch strings.ToLower(path) {
	case "stdout":
		return os.Stdout, nil
	case "stderr":
		return os.Stderr, nil
	default:
		if path != "" {
			return &lumberjack.Logger{Filename: path, MaxSize: 1024, MaxBackups: 30, MaxAge: 7}, nil
		}
		return defaultWriter, nil
	}
}

func mustParseToWriter(path string, defaultWriter io.Writer) io.Writer {
	w, err := parseToWriter(path, defaultWriter)
	if err != nil {
		panic(err)
	}
	return w
}

func parseToSlogLevel(stringLevel string) slog.Level {
	var level = slog.LevelDebug
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
