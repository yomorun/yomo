# YoMo ![Go](https://github.com/yomorun/yomo/workflows/Go/badge.svg)

YoMo is an open-source Streaming Serverless Framework for building Low-latency Edge Computing applications. Built atop QUIC Transport Protocol and Functional Reactive Programming interface, it makes real-time data processing reliable, secure, and easy.

More info at [https://yomo.run](https://yomo.run/?utm_source=github&utm_campaign=ossc) <a href="https://vercel.com/?utm_source=cella&utm_campaign=oss" target="_blank"><img src="https://raw.githubusercontent.com/abumalick/powered-by-vercel/master/powered-by-vercel.svg" height="25px" /></a>

[🇨🇳中文](https://gitee.com/yomorun/yomo)

## Getting Started

### 1. Install yomo CLI

```bash
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/yomorun/install/HEAD/install.sh)"
```

### 2. Create app.go

```bash
mkdir yomo-demo && cd $_ && touch app.go
```

Write your `app.go` code:

```go 
ppackage main

import (
	"context"
	"fmt"
	"time"

	"github.com/yomorun/yomo/pkg/rx"
)

var printer = func(_ context.Context, i interface{}) (interface{}, error) {
	value := i.(float32)
	fmt.Println("serverless get value:", value)
	return value, nil
}

// Handler will handle data in Rx way
func Handler(rxstream rx.RxStream) rx.RxStream {
	stream := rxstream.
		Y3Decoder("0x10", float32(0)).
		AuditTime(100 * time.Millisecond).
		Map(printer).
		StdOut()

	return stream
}
```

### 3. Build and run

1. Run `yomo dev` from the terminal. you will see the following message:

```bash
(20:08:50 ~/yomo/examples)──> yomo dev
2020/12/18 20:09:12 Building the Serverless Function File...
2020/12/18 20:09:14 ✅ Listening on 0.0.0.0:4242
```
Congratulations! You have done your first YoMo application.

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

Any questions or good ideas, please feel free to come to our [Discussion](https://github.com/yomorun/yomo/discussions). Any feedback would be greatly appreciated!

## License

[Apache License 2.0](http://www.apache.org/licenses/LICENSE-2.0.html)
