## YoMo ![Go](https://github.com/yomorun/yomo/workflows/Go/badge.svg)

YoMo is an open source project for building your own IoT edge computing applications. With YoMo, you can speed up the development of microservices-based applications, and your industrial IoT platform will take full advantage of the low latency and high bandwidth brought by 5G.

More info at [yomo.run](https://yomo.run/).

## Getting Started

### 1. Install the current release

Create a directory named `yomotest` and `cd` into it.

	mkdir yomotest
	cd yomotest

Make the current directory the root of a module by using `go mod init`.

	go mod init yomotest

Download and install.

	go get -u github.com/yomorun/yomo

### 2. Create file `echo.go`

To check that YoMo is installed correctly on your device, create a file named `echo.go` and copy the following code to your file:

```rust
package main

// import yomo
import (
	"github.com/yomorun/yomo/pkg/yomo"
)

func main() {
	// 运行该Plugin，监听:port 4241, data will be sent by yomo egde
	// yomo.Run(&EchoPlugin{}, "0.0.0.0:4241")
	
	// 开发调试时的方法, when connected to the Internet, it will automatically connect to the development server of yomo.run 会自动连接至 yomo.run 的开发服务器
	// 连接成功后，该Plugin会每2秒收到一条Observed()方法指定的Key的Value
	yomo.RunDev(&EchoPlugin{}, "localhost:4241")
}

// EchoPlugin - a yomo plugin，会将接受到的数据转换成String形式，并再结尾添加内容，修改
// 后的数据将流向下一个Plugin
type EchoPlugin struct{}

// Handle - 方法将会在数据流入时被执行，使用Observed()方法通知YoMo该Plugin要关注的key，参数value
// 即该Plugin要处理的内容
func (p *EchoPlugin) Handle(value interface{}) (interface{}, error) {
	return value.(string) + "✅", nil
}

// Observed - returns a value of type string, which 该值是EchoPlugin插件关注的数据流中的Key，该数据流中Key对应
// 的Value将会以对象的形式被传递进Handle()方法中
// the corresponding value will be passed into the Handle() method as an object
func (p EchoPlugin) Observed() string {
	return "name"
}

// Name - sets the name of a given plugin p (mainly used for debugging)
func (p *EchoPlugin) Name() string {
	return "EchoPlugin"
}
```

### 3. Build and run

1. Run `go run echo.go` from the terminal. If YoMo is installed successfully, you will see the following message:

```bash
% go run a.go
[EchoPlugin:6031]2020/07/06 22:14:20 plugin service start... [localhost:4241]
name:yomo!✅
name:yomo!✅
name:yomo!✅
name:yomo!✅
name:yomo!✅
^Csignal: interrupt
```
Congratulations! You have written and tested your first YoMo app.

## 🌟 YoMo架构和亮点

![yomo-arch](https://yomo.run/yomo-arch.png)

### YoMo关注在：

- industrial IoT:
	- 在IoT device接入侧, real-time communication with a latency of less than 10ms is required
	- 在智能设备侧，需要在边缘侧进行大算力的AI执行工作
- YoMo is consisted of 2 important parts：
	- `yomo-edge`: 部署在企业内网，负责接收设备数据，并按照配置，依次执行各个`yomo-plugin`
	- `yomo-plugin`: 可以部署在企业私有云、公有云及`yomo-edge-server`上

### Why YoMo

- Based on QUIC (Quick UDP Internet Connection) protocol for data transmission, which uses the User Datagram Protocol (UDP) as its basis instead of the Transmission Control Protocol (TCP); significantly improves the stability and 高通率 of data transmission
- A self-developed `yomo-codec` optimizes decoding performance. For more information, visit [its own repository](https://github.com/yomorun/yomo-codec) on GitHub.
- Based on stream computing, which improves speed and accuracy when dealing with data handling and analysis; simplifies the complexity of stream-oriented programming.

## Contributing

First off, thank you for considering making contributions. It's people like you that make YoMo better. There are many ways in which you can participate in the project, for example:

- File a [bug report](https://github.com/yomorun/yomo/issues/new?assignees=&labels=bug&template=bug_report.md&title=%5BBUG%5D). Be sure to include information like what version of YoMo you are using, what your operating system is, and steps to recreate the bug.

- Suggest a new feature.

- Read our [contributing guidelines](https://github.com/yomorun/yomo/blob/master/CONTRIBUTING.md) to learn about what types of contributions we are looking for.

- We have also adopted a [code of conduct](https://github.com/yomorun/yomo/blob/master/CODE_OF_CONDUCT.md) that we expect project participants to adhere to.

## Feedback

Email us at [yomo@cel.la](mailto:yomo@cel.la). Any feedback would be greatly appreciated!

## License

[Apache License 2.0](http://www.apache.org/licenses/LICENSE-2.0.html)
