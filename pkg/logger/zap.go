package logger

import (
	"log"

	"go.uber.org/zap"
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
	// cfg.DisableStacktrace = true

	if isJsonFormat() {
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

func (z zapLogger) Info(msg string, fields ...interface{}) {
	z.logger.Infow(msg, fields...)
}

func (z zapLogger) Warn(msg string, fields ...interface{}) {
	z.logger.Warnw(msg, fields...)
}

func (z zapLogger) Error(msg string, fields ...interface{}) {
	z.logger.Errorw(msg, fields...)
}

func (z zapLogger) Panic(msg string, fields ...interface{}) {
	z.logger.Panicw(msg, fields...)
}

func (z zapLogger) Fatal(msg string, fields ...interface{}) {
	z.logger.Fatalw(msg, fields...)
}
