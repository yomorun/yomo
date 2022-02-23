package log

// Level of log
type Level uint8

const (
	// DebugLevel defines debug log level.
	DebugLevel Level = iota + 1
	// InfoLevel defines info log level.
	InfoLevel
	// WarnLevel defines warn log level.
	WarnLevel
	// ErrorLevel defines error log level.
	ErrorLevel
	// NoLevel defines an absent log level.
	NoLevel Level = 254
	// Disabled disables the logger.
	Disabled Level = 255
)

// Logger is the interface for logger.
type Logger interface {
	// SetLevel sets the logger level
	SetLevel(Level)
	// SetEncoding sets the logger's encoding
	SetEncoding(encoding string)
	// Printf logs a message wihout level
	Printf(template string, args ...interface{})
	// Debugf logs a message at DebugLevel
	Debugf(template string, args ...interface{})
	// Infof logs a message at InfoLevel
	Infof(template string, args ...interface{})
	// Warnf logs a message at WarnLevel
	Warnf(template string, args ...interface{})
	// Errorf logs a message at ErrorLevel
	Errorf(template string, args ...interface{})
	// Output file path to write log message
	Output(file string)
	// ErrorOutput file path to write error message
	ErrorOutput(file string)
}

// String the logger level
func (l Level) String() string {
	switch l {
	case DebugLevel:
		return "DEBUG"
	case ErrorLevel:
		return "ERROR"
	case WarnLevel:
		return "WARN"
	case InfoLevel:
		return "INFO"
	default:
		return ""
	}
}
