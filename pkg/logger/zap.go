package logger

import (
	stdlog "log"
	"os"
	"time"

	"github.com/yomorun/yomo/core/log"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	timeFormat = "2006-01-02 15:04:05.000"
)

func newLogger(isDebug bool) log.Logger {
	cfg := initConfig()
	if isDebug {
		cfg.Development = true
		cfg.Encoding = "console"
		// set the minimal level to debug
		cfg.Level.SetLevel(zap.DebugLevel)
	} else {
		// set the minimal level to error
		cfg.Level.SetLevel(zap.ErrorLevel)
	}

	if lvl := logLevel(); lvl != "" {
		switch lvl {
		case "debug":
			cfg.Level.SetLevel(zap.DebugLevel)
		case "info":
			cfg.Level.SetLevel(zap.InfoLevel)
		case "warn":
			cfg.Level.SetLevel(zap.WarnLevel)
		case "error":
			cfg.Level.SetLevel(zap.ErrorLevel)
		case "dpanic":
			cfg.Level.SetLevel(zap.DPanicLevel)
		case "panic":
			cfg.Level.SetLevel(zap.PanicLevel)
		case "fatal":
			cfg.Level.SetLevel(zap.FatalLevel)
		}
	}

	if isJSONFormat() {
		cfg.Encoding = "json"
	} else {
		cfg.Encoding = "console"
	}

	logger, err := cfg.Build()
	if err != nil {
		panic(err)
	}

	return zapLogger{
		logger: logger.Sugar(),
	}
}

func initConfig() zap.Config {
	// std logger
	stdlog.Default().SetFlags(0)
	stdlog.Default().SetOutput(new(logWriter))
	// zap config
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     timeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
	return zap.Config{
		Level:             zap.NewAtomicLevelAt(zap.InfoLevel),
		Development:       false,
		DisableCaller:     true,
		DisableStacktrace: true,
		Encoding:          "console",
		EncoderConfig:     encoderConfig,
		OutputPaths:       []string{"stderr"},
		ErrorOutputPaths:  []string{"stderr"},
	}
}
func newLoggerWithConfig(cfg zap.Config) zapLogger {
	logger, err := cfg.Build()
	if err != nil {
		panic(err)
	}

	return zapLogger{
		logger: logger.Sugar(),
	}
}

// zapLogger is the logger implementation in go.uber.org/zap
type zapLogger struct {
	logger      *zap.SugaredLogger
	errorOutput string
}

func (z zapLogger) SetLevel(lvl log.Level) {
	cfg := initConfig()
	switch lvl {
	case log.LevelDebug:
		cfg.Level.SetLevel(zap.DebugLevel)
	case log.LevelInfo:
		cfg.Level.SetLevel(zap.InfoLevel)
	case log.LevelWarn:
		cfg.Level.SetLevel(zap.WarnLevel)
	case log.LevelError:
		cfg.Level.SetLevel(zap.ErrorLevel)
	}

	z = newLoggerWithConfig(cfg)
}

func (z zapLogger) WithPrefix(prefix string) log.Logger {
	// TODO:
	return z
}

func (z zapLogger) ErrorOutput(file string) {
	// TODO:
}

func (z zapLogger) Printf(format string, v ...interface{}) {
	stdlog.Printf(format, v...)
}

func (z zapLogger) Debugf(template string, args ...interface{}) {
	z.logger.Debugf(template, args...)
}

func (z zapLogger) Infof(template string, args ...interface{}) {
	z.logger.Infof(template, args...)
}

func (z zapLogger) Warnf(template string, args ...interface{}) {
	z.logger.Warnf(template, args...)
}

func (z zapLogger) Errorf(template string, args ...interface{}) {
	z.logger.Errorf(template, args...)
}

func timeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format(timeFormat))
}

type logWriter struct{}

func (l logWriter) Write(bytes []byte) (int, error) {
	os.Stderr.WriteString(time.Now().Format(timeFormat))
	os.Stderr.Write([]byte("\t"))
	return os.Stderr.Write(bytes)
}
