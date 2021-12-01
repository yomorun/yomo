package logger

import (
	"log"
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	timeFormat = "2006-01-02 15:04:05.000"
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
	cfg.EncoderConfig.EncodeTime = timeEncoder
	// std logger
	log.Default().SetFlags(0)
	log.Default().SetOutput(new(logWriter))

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

func timeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format(timeFormat))
}

type logWriter struct{}

func (l logWriter) Write(bytes []byte) (int, error) {
	os.Stderr.WriteString(time.Now().Format(timeFormat))
	os.Stderr.Write([]byte("\t"))
	return os.Stderr.Write(bytes)
}
