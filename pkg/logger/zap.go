package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func newLogger(isDebug bool) Logger {
	var cfg zap.Config
	if isDebug {
		cfg = zap.NewDevelopmentConfig()
	} else {
		cfg = zap.NewProductionConfig()
	}

	cfg.EncoderConfig.CallerKey = zapcore.OmitKey
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
