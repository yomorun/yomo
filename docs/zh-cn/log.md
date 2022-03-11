## 应用程序日志处理

### 日志

应用程序可以使用日志记录器，记录日志消息，我们定义了一个日志接口 `logger` 

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

更详细的说明可以查看文件：`core/log/logger.go`

我们提供了一个日志记录器的默认实现，您可以直接引用`github.com/yomorun/yomo/pkg/logger` 包使用，如果默认实现不能满足您的要求，你可以实现上面的接口，然后在编写应用时使用 `yomo.WithLogger ` 选项，例：

```go
sfn := yomo.NewStreamFunction(
	"Name",
  ....
  yomo.WithLogger(customLogger), // customLogger 是您自己的日志记录器实现
)
```

#### 主要方法

- `Printf` 无视日志级别设置，输出日志消息
- `Debugf` 输出调试消息
- `Infof` 输出通知消息
- `Warnf` 输出警告消息
- `Errorf` 输出错误消息

**使用示例：**

```go
import "github.com/yomorun/yomo/pkg/logger"
...
logger.Infof("%s doesn't grow on trees. ","Money")
```

#### 环境变量

- `YOMO_LOG_LEVEL`  设置日志级别，默认：`error`，可选值如下：
  - debug
  - info
  - warn
  - error
- `YOMO_LOG_OUTPUT` 设置日志输出文件，默认不输出
- `YOMO_LOG_ERROR_OUTPUT` 设置发生错误时，将消息输出到指定文件，默认不输出  
- `YOMO_DEBUG_FRAME_SIZE`  设置调试模式下输出`Frame`大小，默认 16 个字节                   

