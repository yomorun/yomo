package logger

import (
	"log"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func newLogger(isDebug bool) Logger {
	var cfg zap.Config
	if isDebug {
		cfg = zap.NewDevelopmentConfig()
		// set the minimal level to debug
		cfg.Level.SetLevel(zap.DebugLevel)
	} else {
		cfg = zap.NewProductionConfig()
		// set the minimal level to error
		cfg.Level.SetLevel(zap.ErrorLevel)
	}

	cfg.DisableCaller = true
	cfg.DisableStacktrace = true
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	if isJSONFormat() {
		cfg.Encoding = "json"
	} else {
		cfg.Encoding = "console"
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
	logger *zap.SugaredLogger
}

func (z zapLogger) Print(v ...interface{}) {
	log.Print(v...)
}

func (z zapLogger) Printf(format string, v ...interface{}) {
	log.Printf(format, v...)
}

func (z zapLogger) Debug(msg string, fields ...interface{}) {
	z.logger.Debugw(msg, fields...)
}

func (z zapLogger) Debugf(template string, args ...interface{}) {
	z.logger.Debugf(template, args...)
}

func (z zapLogger) Info(msg string, fields ...interface{}) {
	z.logger.Infow(msg, fields...)
}

func (z zapLogger) Infof(template string, args ...interface{}) {
	z.logger.Infof(template, args...)
}

func (z zapLogger) Warn(msg string, fields ...interface{}) {
	z.logger.Warnw(msg, fields...)
}

func (z zapLogger) Warnf(template string, args ...interface{}) {
	z.logger.Warnf(template, args...)
}

func (z zapLogger) Error(msg string, fields ...interface{}) {
	z.logger.Errorw(msg, fields...)
}

func (z zapLogger) Errorf(template string, args ...interface{}) {
	z.logger.Errorf(template, args...)
}

func (z zapLogger) Panic(msg string, fields ...interface{}) {
	z.logger.Panicw(msg, fields...)
}

func (z zapLogger) Panicf(template string, args ...interface{}) {
	z.logger.Panicf(template, args...)
}

func (z zapLogger) Fatal(msg string, fields ...interface{}) {
	z.logger.Fatalw(msg, fields...)
}

func (z zapLogger) Fatalf(template string, args ...interface{}) {
	z.logger.Fatalf(template, args...)
}
