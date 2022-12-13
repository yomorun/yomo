## 应用程序日志处理


应用程序可以使用日志记录器，记录日志消息。

yomo 推荐使用 [slog](https://pkg.go.dev/golang.org/x/exp/slog) 打印**结构化日志**

结构化日志: 是一种人类可读的，并且机器可读的日志，通常是键-值对结构，或者 json 结构。

更详细的说明可以查看文件：`core/ylog/logger.go`

Yomo 提供了一个日志记录器的默认实现，默认的 logger 是从环境变量中加载配置。

如果默认实现不能满足你的要求，你也可以直接引用 `slog` 包使用，
或者你也可以实现 `slog.Handler` 接口，然后在编写应用时使用 `yomo.WithLogger ` 选项，例：

```go
sfn := yomo.NewStreamFunction(
	"Name",
  ....
  yomo.WithLogger(customLogger), // customLogger 是你自己的日志记录器实现
)
```

#### 环境变量

- `YOMO_LOG_LEVEL`  设置日志级别，默认：`info`，可选值如下：
  - debug
  - info
  - warn
  - error
- `YOMO_LOG_OUTPUT` 设置日志输出文件，默认输出到 stdout
- `YOMO_LOG_ERROR_OUTPUT` 设置发生错误时，将消息输出到指定文件，默认输出到 stderr
- `YOMO_DEBUG_FRAME_SIZE`  设置调试模式下输出`Frame`大小，默认 16 个字节                   
- `YOMO_LOG_VERBOSE` 设置是否打开 log 的 debug 模式，debug 模式下，日志会输出打印日志的代码行数，不建议在生产环境打开，默认是 false
