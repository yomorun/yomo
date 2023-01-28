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
- `YOMO_LOG_VERBOSE` enable or disable the debug mode of logger, logger outputs the source code position of the log statement if enable it, Do not enable it in production, default is false
- `YOMO_LOG_FORMAT` Format supports text and json, The default is text
- `YOMO_LOG_MAX_SIZE` MaxSize is the maximum size in megabytes of the log file before it gets rotated. It defaults to 100 megabytes
- `YOMO_LOG_MAX_BACKUPS` MaxBackups is the maximum number of old log files to retain. The default is to retain all old log files (though MaxAge may still cause them to get deleted.)
- `YOMO_LOG_MAX_AGE` MaxAge is the maximum number of days to retain old log files based on the timestamp encoded in their filename. The default is not to remove old log files based on age
- `YOMO_LOG_LOCAL_TIME` LocalTime determines if the time used for formatting the timestamps in backup files is the computer's local time. The default is to use UTC time
- `YOMO_LOG_COMPRESS` Compress determines if the rotated log files should be compressed using gzip. The default is not to perform compression
