# YoMo ![Go](https://github.com/yomorun/yomo/workflows/Go/badge.svg)

YoMo is an open-source project for building your own IoT edge computing applications. With YoMo, you can speed up the development of microservices-based applications, and your industrial IoT platform will take full advantage of the low latency and high bandwidth brought by 5G.

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
	// run echo plugin and monitor port 4241; data will be sent by yomo egde
	// yomo.Run(&EchoPlugin{}, "0.0.0.0:4241")
	
	// a method for development and testing; when connected to the Internet, it will
	// automatically connect to the development server of yomo.run
	// after successfully connected to the server, the plugin will receive the value
	// of the key specified by the Observed() method every 2 seconds
	yomo.RunDev(&EchoPlugin{}, "localhost:4241")
}

// EchoPlugin - a yomo plugin that converts received data into strings and appends
// additional information to the strings; the modified data will flow to the next plugin
type EchoPlugin struct{}

// Handle - this method will be called when data flows in; the Observed() method is used
// to tell yomo which key the plugin should monitor; the parameter value is what the plugin
// needs to process
func (p *EchoPlugin) Handle(value interface{}) (interface{}, error) {
	return value.(string) + "✅", nil
}

// Observed - returns a value of type string, which is the key monitored by echo plugin;
// the corresponding value will be passed into the Handle() method as an object
func (p EchoPlugin) Observed() string {
	return "name"
}

// Name - sets the name of a given plugin p (mainly used for debugging)
func (p *EchoPlugin) Name() string {
	return "EchoPlugin"
}

// Mold describe the struct of `Observed` value
func (p EchoPlugin) Mold() interface{} {
	return ""
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

Note: If you want to use a complex Mold, please refer to  [yomo-echo-plugin](https://github.com/yomorun/yomo-echo-plugin).

## Illustration

![yomo-arch](https://yomo.run/yomo-arch.png)

### YoMo focuses on：

- Industrial IoT:
	- On the IoT device side, real-time communication with a latency of less than 10ms is required.
	- On the smart device side, AI performing with a high hash rate is required.
- YoMo consists of 2 parts：
	- `yomo-edge`: deployed on company intranet; responsible for receiving device data and executing each yomo-plugin in turn according to the configuration
	- `yomo-plugin`: can be deployed on public cloud, private cloud, and `yomo-edge-server`

### Why YoMo

- Based on QUIC (Quick UDP Internet Connection) protocol for data transmission, which uses the User Datagram Protocol (UDP) as its basis instead of the Transmission Control Protocol (TCP); significantly improves the stability and throughput of data transmission.
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
