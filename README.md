<p align="center">
  <img width="200px" height="200px" src="https://blog.yomo.run/static/images/logo.png" />
</p>

# YoMo ![Go](https://github.com/yomorun/yomo/workflows/Go/badge.svg) [![Discord](https://img.shields.io/discord/770589787404369930.svg?label=discord&logo=discord&logoColor=ffffff&color=7389D8&labelColor=6A7EC2)](https://discord.gg/RMtNhx7vds)

YoMo is an open-source Streaming Serverless Framework for building Low-latency Edge Computing applications. Built atop QUIC Transport Protocol and Functional Reactive Programming interface. makes real-time data processing reliable, secure, and easy.

Official Website: 🦖[https://yomo.run](https://yomo.run)

[Gitee](https://gitee.com/yomorun/yomo)

## 🌶 Features

|     | **Features**|
| --- | ----------------------------------------------------------------------------------|
| ⚡️  | **Low-latency** Guaranteed by implementing atop QUIC [QUIC](https://datatracker.ietf.org/wg/quic/documents/) |
| 🔐  | **Security** TLS v1.3 on every data packet by design |
| 📱  | **5G/WiFi-6** Reliable networking in Celluar/Wireless |
| 🌎  | **Geo-Distributed Edge Mesh** Edge-Mesh Native architecture makes your services close to end users |
| 📸  | **Event-First** Architecture leverages serverless service to be event driven and elastic  |
| 🦖  | **Streaming Serverless** Write only a few lines of code to build applications and microservices |
| 🚀  | **Y3** a [faster than real-time codec](https://github.com/yomorun/y3-codec-golang) |
| 📨  | **Reactive** stream processing based on [Rx](http://reactivex.io/documentation/operators.html) |

## 🚀 Getting Started

### 1. Install CLI

> **Note:** YoMo requires Go 1.15 and above, run `go version` to get the version of Go in your environment, please follow [this link](https://golang.org/doc/install) to install or upgrade if it doesn't fit the requirement.

```bash
# Ensure use $GOPATH, golang requires main and plugin highly coupled
○ echo $GOPATH

```

if `$GOPATH` is not set, check [Set $GOPATH and $GOBIN](#optional-set-gopath-and-gobin) first.

```bash
$ GO111MODULE=off go get github.com/yomorun/yomo

$ cd $GOPATH/src/github.com/yomorun/yomo

$ make install
```

![YoMo Tutorial 1](https://yomo.run/tutorial-1.png)

### 2. Create your serverless app

```bash
$ mkdir -p $GOPATH/src/github.com/{YOUR_GITHUB_USERNAME} && cd $_

$ yomo init yomo-app-demo
2020/12/29 13:03:57 Initializing the Serverless app...
2020/12/29 13:04:00 ✅ Congratulations! You have initialized the serverless app successfully.
2020/12/29 13:04:00 🎉 You can enjoy the YoMo Serverless via the command: yomo dev

$ cd yomo-app-demo

```

![YoMo Tutorial 2](https://yomo.run/tutorial-2.png)

CLI will automatically create the `app.go`:

```go
package main

import (
	"context"
	"fmt"
	"time"

	y3 "github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo/pkg/rx"
)

// NoiseDataKey represents the Tag of a Y3 encoded data packet
const NoiseDataKey = 0x10

// NoiseData represents the structure of data
type NoiseData struct {
	Noise float32 `y3:"0x11"`
	Time  int64   `y3:"0x12"`
	From  string  `y3:"0x13"`
}

var printer = func(_ context.Context, i interface{}) (interface{}, error) {
	value := i.(NoiseData)
	rightNow := time.Now().UnixNano() / int64(time.Millisecond)
	fmt.Println(fmt.Sprintf("[%s] %d > value: %f ⚡️=%dms", value.From, value.Time, value.Noise, rightNow-value.Time))
	return value.Noise, nil
}

var callback = func(v []byte) (interface{}, error) {
	var mold NoiseData
	err := y3.ToObject(v, &mold)
	if err != nil {
		return nil, err
	}
	mold.Noise = mold.Noise / 10
	return mold, nil
}

// Handler will handle data in Rx way
func Handler(rxstream rx.RxStream) rx.RxStream {
	stream := rxstream.
		Subscribe(NoiseDataKey).
		OnObserve(callback).
		Debounce(50).
		Map(printer).
		StdOut()

	return stream
}

```

### 3. Build and run

1. Run `yomo dev` from the terminal. you will see the following message:

![YoMo Tutorial 3](https://yomo.run/tutorial-3.png)

Congratulations! You have done your first YoMo application.

### Optional: Set $GOPATH and $GOBIN

for current session:

```bash
export GOPATH=~/.go
export PATH=$GOPATH/bin:$PATH
```

for shell: 

for `zsh` users

```bash
echo "export GOPATH=~/.go" >> .zshrc
echo "path+=$GOPATH/bin" >> .zshrc
```

for `bash` users

```bash
echo 'export GOPATH=~/.go' >> .bashrc
echo 'export PATH="$GOPATH/bin:$PATH"' >> ~/.bashrc
```

## 🧩 Interop

### event-first processing

[Multiple data sources combined calculation](https://github.com/yomorun/yomo/tree/master/example/trainingmodel)

### Sources

+ [Connect EMQ X Broker to YoMo](https://github.com/yomorun/yomo-source-emqx-starter)
+ [Connect MQTT to YoMo](https://github.com/yomorun/yomo-source-mqtt-broker-starter)

### Flows

+ [Write a YoMo-Flow with WebAssembly by SSVM](https://github.com/yomorun/yomo-flow-ssvm-example)

### Sinks

+ [Connect to FaunaDB to store post-processed result the serverless way](https://github.com/yomorun/yomo-sink-faunadb-example)
+ Connect to InfluxDB to store post-processed result
+ [Connect to TDEngine to store post-processed result](https://github.com/yomorun/yomo-sink-tdengine-example)

## 🗺 Location Insensitive Deployment

![yomo-flow-arch](https://yomo.run/yomo-flow-arch.jpg)

## 📚 Documentation

+ `YoMo-Source`: [yomo.run/source](https://yomo.run/source)
+ `YoMo-Flow`: [yomo.run/flow](https://yomo.run/flow)
+ `YoMo-Sink`: [yomo.run/sink](https://yomo.run/sink)
+ `YoMo-Zipper`: [yomo.run/zipper](https://yomo.run/zipper)
+ `Stream Processing in Rx way`: [Rx](https://yomo.run/rx)
+ `Faster than real-time codec`: [Y3](https://github.com/yomorun/y3-codec)

[YoMo](https://yomo.run) ❤️ [Vercel](https://vercel.com/?utm_source=yomorun&utm_campaign=oss), Our documentation website is

[![Vercel Logo](https://yomo.run/vercel.svg)](https://vercel.com/?utm_source=yomorun&utm_campaign=oss)

## 🎯 Focuses on computings out of data center

- IoT/IIoT/AIoT
- Latency-sensitive applications.
- Networking situation with packet loss or high latency.
- Handling continuous high frequency generated data with stream-processing.
- Building Complex systems with Streaming-Serverless architecture.

## 🌟 Why YoMo

- Based on QUIC (Quick UDP Internet Connection) protocol for data transmission, which uses the User Datagram Protocol (UDP) as its basis instead of the Transmission Control Protocol (TCP); significantly improves the stability and throughput of data transmission. Especially for cellular networks like 5G.
- A self-developed `y3-codec` optimizes decoding performance. For more information, visit [its own repository](https://github.com/yomorun/y3-codec) on GitHub.
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
