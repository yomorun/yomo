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

### Prerequisite

[Install Go](https://golang.org/doc/install)

### 1. Install CLI

```bash
$ go install github.com/yomorun/cli/yomo@latest
```

#### Verify if the CLI was installed successfully

```bash
$ yomo -V

YoMo CLI version: v0.0.6
```

### 2. Create your stream function

```bash
$ yomo init yomo-app-demo

⌛  Initializing the Stream Function...
✅  Congratulations! You have initialized the stream function successfully.
ℹ️   You can enjoy the YoMo Stream Function via the command: 
ℹ️   	DEV: 	yomo dev -n Noise yomo-app-demo/app.go
ℹ️   	PROD: 	First run source application, eg: go run example/source/main.go
		Second: yomo run -n yomo-app-demo yomo-app-demo/app.go

$ cd yomo-app-demo

```

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
func Handler(rxstream rx.Stream) rx.Stream {
	stream := rxstream.
		Subscribe(NoiseDataKey).
		OnObserve(callback).
		Debounce(50).
		Map(printer).
		StdOut().
		Encode(0x11)

	return stream
}

```

### 3. Build and run

1. Run `yomo dev` from the terminal. you will see the following message:

```sh
$ yomo dev

ℹ️   YoMo Stream Function file: app.go
⌛  Create YoMo Stream Function instance...
⌛  YoMo Stream Function building...
✅  Success! YoMo Stream Function build.
ℹ️   YoMo Stream Function is running...
2021/06/07 12:00:06 Connecting to yomo-server dev.yomo.run:9000 ...
2021/06/07 12:00:07 ✅ Connected to yomo-server dev.yomo.run:9000
[10.10.79.50] 1623038407236 > value: 1.919251 ⚡️=-25ms
[StdOut]:  1.9192511
[10.10.79.50] 1623038407336 > value: 11.370256 ⚡️=-25ms
[StdOut]:  11.370256
[10.10.79.50] 1623038407436 > value: 8.672209 ⚡️=-25ms
[StdOut]:  8.672209
[10.10.79.50] 1623038407536 > value: 4.826996 ⚡️=-25ms
[StdOut]:  4.826996
[10.10.79.50] 1623038407636 > value: 16.201773 ⚡️=-25ms
[StdOut]:  16.201773
[10.10.79.50] 1623038407737 > value: 13.875483 ⚡️=-26ms
[StdOut]:  13.875483

```

Congratulations! You have done your first YoMo Stream Function.


## 🧩 Interop

### event-first processing

[Multiple data sources combined calculation](https://github.com/yomorun/yomo/tree/master/example/trainingmodel)

### Sources

+ [Connect EMQ X Broker to YoMo](https://github.com/yomorun/yomo-source-emqx-starter)
+ [Connect MQTT to YoMo](https://github.com/yomorun/yomo-source-mqtt-broker-starter)

### Stream Functions

+ [Write a Stream Function with WebAssembly by SSVM](https://github.com/yomorun/yomo-flow-ssvm-example)

### Output Connectors

+ [Connect to FaunaDB to store post-processed result the serverless way](https://github.com/yomorun/yomo-sink-faunadb-example)
+ Connect to InfluxDB to store post-processed result
+ [Connect to TDEngine to store post-processed result](https://github.com/yomorun/yomo-sink-tdengine-example)

## 🗺 Location Insensitive Deployment

![yomo-flow-arch](https://docs.yomo.run/yomo-flow-arch.jpg)

## 📚 Documentation

+ `YoMo-Source`: [yomo.run/source](https://docs.yomo.run/source)
+ `YoMo-Stream-Function` (formerly flow): [yomo.run/stream-function](https://docs.yomo.run/flow)
+ `YoMo-Output-Connector` (formerly sink): [yomo.run/output-connector](https://docs.yomo.run/sink)
+ `YoMo-Server` (formerly zipper): [yomo.run/yomo-server](https://docs.yomo.run/zipper)
+ `Stream Processing in Rx way`: [Rx](https://docs.yomo.run/rx)
+ `Faster than real-time codec`: [Y3](https://github.com/yomorun/y3-codec)

[YoMo](https://yomo.run) ❤️ [Vercel](https://vercel.com/?utm_source=yomorun&utm_campaign=oss), Our documentation website is

[![Vercel Logo](https://docs.yomo.run/vercel.svg)](https://vercel.com/?utm_source=yomorun&utm_campaign=oss)

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
