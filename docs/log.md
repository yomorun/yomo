## Application Log 

Applications can use loggers to record log messages.

yomo suggests using [slog](https://pkg.go.dev/golang.org/x/exp/slog) to output **structured log**

Structured logging is the ability to output logs with machine-readable structure, typically key-value pairs or json, in addition to a human-readable message.

More detailed instructions can be found in the documentation:`core/ylog/logger.go`

Yomo provide a default implementation of the logger, The default loads config from environment.

If the default implementation can not meet your requirements,
you can import `slog` directly, you can also implement interface to `slog.Handler`, and then use the `yomo.WithLogger ` option , for example:

```go
sfn  := yomo.NewStreamFunction (
	 "Name" ,
   ... .
   yomo .WithLogger ( customLogger ), // customLogger is your own logger implementation 
)
```

#### Environment Variables

- `YOMO_LOG_LEVEL`   Set the log level, default:  `info` , optional values are as follows:
  - debug
  - info
  - warn
  - error
  
- `YOMO_LOG_OUTPUT` Set the log output file, the default is stdout

- `YOMO_LOG_ERROR_OUTPUT` When an error occurs, output the message to the specified file, the default is stderr

- `YOMO_DEBUG_FRAME_SIZE` Set the output size in debug mode `Frame`, the default is 16 bytes
- - `YOMO_LOG_VERBOSE` enable or disable the debug mode of logger, logger outputs the source code position of the log statement if enable it, Do not enable it in production, default is false
