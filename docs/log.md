## Application Log 

### Log

Applications can use loggers to record log messages, we define a logging interface`logger`

```go
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
```

More detailed instructions can be found in the documentation:`core/log/logger.go`

We provide a default implementation of the logger, you can directly refer to the `github.com/yomorun/yomo/pkg/logger` package to use, if the default implementation can not meet your requirements, you can implement the above interface, and then use the `yomo.WithLogger ` option , for example:

```go
sfn  := yomo.NewStreamFunction (
	 "Name" ,
   ... .
   yomo .WithLogger ( customLogger ), // customLogger is your own logger implementation 
)
```

#### Methods:

- `Printf` Output log messages regardless of log level settings
- `Debugf` Output debug messages
- `Infof` Output information message
- `Warnf` Output warning message
- `Errorf` Output error message

**Example of use:**

```go
import "github.com/yomorun/yomo/pkg/logger"
...
logger.Infof("%s doesn't grow on trees. ","Money")
```

#### Environment Variables

- `YOMO_LOG_LEVEL`   Set the log level, default:  `error` , optional values are as follows:
  - debug
  - info
  - warn
  - error
  
- `YOMO_LOG_OUTPUT` Set the log output file, the default is not output

- `YOMO_LOG_ERROR_OUTPUT` When an error occurs, output the message to the specified file, the default is not output

- `YOMO_DEBUG_FRAME_SIZE` Set the output size in debug mode `Frame`, the default is 16 bytes
