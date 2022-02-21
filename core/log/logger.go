package log

// Level of log
type Level uint8

const (
	// Disabled disables the logger.
	Disabled Level = iota
	// LevelDebug defines debug log level.
	LevelDebug
	// ErrorLevel defines error log level.
	LevelError
	// LevelWarn defines warn log level.
	LevelWarn
	// LevelInfo defines info log level.
	LevelInfo
	// LevelNo defines an absent log level.
	LevelNo
)

// Logger is the interface for logger.
type Logger interface {
	SetLevel(Level)
	// SetTimeFormat(format string)
	WithPrefix(prefix string) Logger
	// Printf prints a formated message at LevelNo
	Printf(template string, v ...interface{})
	// Debugf logs a message at LevelDebug.
	Debugf(template string, args ...interface{})
	// Infof logs a message at LevelInfo.
	Infof(template string, args ...interface{})
	// Warnf logs a message at LevelWarn.
	Warnf(template string, args ...interface{})
	// Errorf logs a message at LevelError.
	Errorf(template string, args ...interface{})
	// Output file path to write log message output to
	Output(file string)
	// ErrorOutput file path to write error message output to
	ErrorOutput(file string)
}
