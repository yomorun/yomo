<p align="center">
  <img width="200px" height="200px" src="https://docs.yomo.run/favicon.ico" />
</p>

# YoMo ![Go](https://github.com/yomorun/yomo/workflows/Go/badge.svg)

YoMo is an open-source Streaming Serverless Framework for building Low-latency Edge Computing applications. Built atop QUIC Transport Protocol and Functional Reactive Programming interface. makes real-time data processing reliable, secure, and easy.

More info at 🦖[https://yomo.run]

[简体中文](https://gitee.com/yomorun/yomo)

## 🚀 Getting Started

### 1. Install CLI

```bash
# Ensure use $GOPATH, golang requires main and plugin highly coupled
○ echo $GOPATH

○ GO111MODULE=off go get github.com/yomorun/yomo

○ cd $GOPATH/src/github.com/yomorun/yomo

○ make install

○ which yomo

○ yomo version
yomo version 0.5.1-1-gacaf08a (2020-12-29)
```

### 2. Create your serverless app

```bash
○ mkdir $GOPATH/src/github.com/{YOUR_GITHUB_USERNAME} && cd $_

○ yomo init yomo-app-demo
2020/12/29 13:03:57 Initializing the Serverless app...
2020/12/29 13:04:00 ✅ Congratulations! You have initialized the serverless app successfully.
2020/12/29 13:04:00 🎉 You can enjoy the YoMo Serverless via the command: yomo dev

○ cd yomo-app-demo

```

CLI will automatically create the `app.go`:

```go
package main

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

### 4. Build and run

1. Run `yomo dev` from the terminal. you will see the following message:

```bash
○ yomo dev
2020/12/29 13:24:01 Building the Serverless Function File...
2020/12/29 13:24:02 ✅ Listening on 0.0.0.0:4242
serverless get value: 8.049803
[StdOut]:  8.049803
serverless get value: 24.885649
[StdOut]:  24.885649
serverless get value: 57.41162
[StdOut]:  57.41162
serverless get value: 98.15276
[StdOut]:  98.15276
serverless get value: 176.20674
[StdOut]:  176.20674
serverless get value: 160.75317
[StdOut]:  160.75317
serverless get value: 56.65883
[StdOut]:  56.65883
serverless get value: 192.10464
[StdOut]:  192.10464
serverless get value: 75.196396
```

Congratulations! You have done your first YoMo application.

### Optional: Set $GOPATH and $GOBIN

for current session:

```bash
export PATH=$GOPATH/bin:$PATH
```

for shell: 

for `zsh` users

```bash
echo "path+=$GOPATH/bin" >> .zshrc
```

for `bash` users

```bash
echo 'export PATH="$GOPATH/bin:$PATH"' >> ~/.bashrc
```

## 🎯 Focuses on computings out of data center

- Latency-sensitive applications.
- Networking situation with packet loss or high latency.
- Handling continuous high frequency generated data with stream-processing.
- Building Complex systems with Streaming-Serverless architecture.

## 🌟 Why YoMo

- Based on QUIC (Quick UDP Internet Connection) protocol for data transmission, which uses the User Datagram Protocol (UDP) as its basis instead of the Transmission Control Protocol (TCP); significantly improves the stability and throughput of data transmission. Especially for cellular networks like 5G.
- A self-developed `yomo-codec` optimizes decoding performance. For more information, visit [its own repository](https://github.com/yomorun/yomo-codec) on GitHub.
- Based on stream computing, which improves speed and accuracy when dealing with data handling and analysis; simplifies the complexity of stream-oriented programming.
- Secure-by-default from transport protocol.

## 🦸 Contributing

First off, thank you for considering making contributions. It's people like you that make YoMo better. There are many ways in which you can participate in the project, for example:

- File a [bug report](https://github.com/yomorun/yomo/issues/new?assignees=&labels=bug&template=bug_report.md&title=%5BBUG%5D). Be sure to include information like what version of YoMo you are using, what your operating system is, and steps to recreate the bug.
- Suggest a new feature.
- Read our [contributing guidelines](https://github.com/yomorun/yomo/blob/master/CONTRIBUTING.md) to learn about what types of contributions we are looking for.
- We have also adopted a [code of conduct](https://github.com/yomorun/yomo/blob/master/CODE_OF_CONDUCT.md) that we expect project participants to adhere to.

## 🤹🏻‍♀️ Feedback

Any questions or good ideas, please feel free to come to our [Discussion](https://github.com/yomorun/yomo/discussions). Any feedback would be greatly appreciated!

## License

[Apache License 2.0](http://www.apache.org/licenses/LICENSE-2.0.html)
