package logger

import (
	stdlog "log"
	"os"
	"time"

	"github.com/yomorun/yomo/core/log"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	timeFormat = "2006-01-02 15:04:05.000"
)

// zapLogger is the logger implementation in go.uber.org/zap
type zapLogger struct {
	level       zapcore.Level
	debug       bool
	encoding    string
	opts        []zap.Option
	logger      *zap.Logger
	instance    *zap.SugaredLogger
	output      string
	errorOutput string
}

// Default the default logger instance
func Default(debug ...bool) log.Logger {
	z := New()
	z.SetLevel(logLevel())
	if isJSONFormat() {
		z.SetEncoding("json")
	}
	// env debug
	if isEnableDebug() {
		z.SetLevel(log.DebugLevel)
	}
	if len(debug) > 0 {
		if debug[0] {
			z.SetLevel(log.DebugLevel)
		}
	}
	z.Output(output())
	z.ErrorOutput(errorOutput())

	return z
}

// New create new logger instance
func New(opts ...zap.Option) log.Logger {
	// std logger
	stdlog.Default().SetFlags(0)
	stdlog.Default().SetOutput(new(logWriter))

	z := zapLogger{
		level:    zap.ErrorLevel,
		debug:    false,
		encoding: "console",
		opts:     opts,
	}

	return &z
}

func openSinks(cfg zap.Config) (zapcore.WriteSyncer, zapcore.WriteSyncer, error) {
	sink, closeOut, err := zap.Open(cfg.OutputPaths...)
	if err != nil {
		return nil, nil, err
	}
	errSink, _, err := zap.Open(cfg.ErrorOutputPaths...)
	if err != nil {
		closeOut()
		return nil, nil, err
	}
	return sink, errSink, nil
}

// SetEncoding set logger message coding
func (z *zapLogger) SetEncoding(enc string) {
	z.encoding = enc
}

// SetLevel set logger level
func (z *zapLogger) SetLevel(lvl log.Level) {
	isDebug := lvl == log.DebugLevel
	level := zap.ErrorLevel
	switch lvl {
	case log.DebugLevel:
		level = zap.DebugLevel
	case log.InfoLevel:
		level = zap.InfoLevel
	case log.WarnLevel:
		level = zap.WarnLevel
	case log.ErrorLevel:
		level = zap.ErrorLevel
	}
	z.level = level
	z.debug = isDebug
}

// Output file path to write log message
func (z *zapLogger) Output(file string) {
	if file != "" {
		z.output = file
	}
}

// ErrorOutput file path to write log message
func (z *zapLogger) ErrorOutput(file string) {
	if file != "" {
		z.errorOutput = file
	}
}

// Printf logs a message wihout level
func (z *zapLogger) Printf(format string, v ...interface{}) {
	stdlog.Printf(format, v...)
}

// Debugf logs a message at DebugLevel
func (z *zapLogger) Debugf(template string, args ...interface{}) {
	z.Instance().Debugf(template, args...)
}

// Infof logs a message at InfoLevel
func (z *zapLogger) Infof(template string, args ...interface{}) {
	z.Instance().Infof(template, args...)
}

// Warnf logs a message at WarnLevel
func (z zapLogger) Warnf(template string, args ...interface{}) {
	z.Instance().Warnf(template, args...)
}

// Errorf logs a message at ErrorLevel
func (z zapLogger) Errorf(template string, args ...interface{}) {
	z.Instance().Errorf(template, args...)
}

func (z *zapLogger) Instance() *zap.SugaredLogger {
	if z.instance == nil {
		// zap
		encoderConfig := zapcore.EncoderConfig{
			TimeKey:        "ts",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller",
			FunctionKey:    zapcore.OmitKey,
			MessageKey:     "msg",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.CapitalColorLevelEncoder,
			EncodeTime:     timeEncoder,
			EncodeDuration: zapcore.SecondsDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		}
		cfg := zap.Config{
			Level:             zap.NewAtomicLevelAt(zap.ErrorLevel),
			Development:       z.debug,
			DisableCaller:     true,
			DisableStacktrace: true,
			Encoding:          z.encoding,
			EncoderConfig:     encoderConfig,
			OutputPaths:       []string{"stderr"},
			ErrorOutputPaths:  []string{"stderr"},
		}
		cfg.Level.SetLevel(z.level)
		if z.debug {
			// set the minimal level to debug
			cfg.Level.SetLevel(zap.DebugLevel)
		}
		// output
		if z.output != "" {
			cfg.OutputPaths = append(cfg.OutputPaths, z.output)
		}
		encoder := zapcore.NewConsoleEncoder(encoderConfig)
		sink, _, err := openSinks(cfg)
		if err != nil {
			panic(err)
		}
		core := zapcore.NewCore(encoder, sink, cfg.Level)
		// error output
		if z.errorOutput != "" {
			rotatedLogger := errorRotatedLogger(z.errorOutput, 10, 30, 7)
			errorOutputOption := zap.Hooks(func(entry zapcore.Entry) error {
				if entry.Level == zap.ErrorLevel {
					msg, err := encoder.EncodeEntry(entry, nil)
					if err != nil {
						return err
					}
					rotatedLogger.Write(msg.Bytes())
				}
				return nil
			})
			z.opts = append(z.opts, errorOutputOption)
		}
		logger := zap.New(core, z.opts...)

		z.logger = logger
		z.instance = z.logger.Sugar()
	}
	return z.instance
}

func errorRotatedLogger(file string, maxSize, maxBacukups, maxAge int) *lumberjack.Logger {
	return &lumberjack.Logger{
		Filename:   file,
		MaxSize:    maxSize,
		MaxBackups: maxBacukups,
		MaxAge:     maxAge,
		Compress:   false,
	}
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
