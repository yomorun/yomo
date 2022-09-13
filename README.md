<p align="center">
  <img width="200px" height="200px" src="https://blog.yomo.run/static/images/logo.png" />
</p>

# YoMo ![Go](https://github.com/yomorun/yomo/workflows/Go/badge.svg) [![Discord](https://img.shields.io/discord/770589787404369930.svg?label=discord&logo=discord&logoColor=ffffff&color=7389D8&labelColor=6A7EC2)](https://discord.gg/RMtNhx7vds)

YoMo is an open-source Streaming Serverless Framework for building Low-latency Edge Computing applications. Built atop QUIC Transport Protocol and Functional Reactive Programming interface, it makes real-time data processing reliable, secure, and easy.

Official Website: 🦖[https://yomo.run](https://yomo.run)

💚 We care about: **The Demand For Real-Time Digital User Experiences**

It’s no secret that today’s users want instant gratification, every productivity application is more powerful when it's collaborative. But, currently, when we talk about `distribution`, it represents **distribution in data center**. API is far away from their users from all over the world.

If an application can be deployed anywhere close to their end users, solve the problem, this is **Geo-distributed System Architecture**:

<img width="580" alt="yomo geo-distributed system" src="https://user-images.githubusercontent.com/65603/162367572-5a0417fa-e2b2-4d35-8c92-2c95d461706d.png">

## 🌶 Features

|     | **Features**|
| --- | ----------------------------------------------------------------------------------|
| ⚡️  | **Low-latency** Guaranteed by implementing atop QUIC [QUIC](https://datatracker.ietf.org/wg/quic/documents/) |
| 🔐  | **Security** TLS v1.3 on every data packet by design |
| 📱  | **5G/WiFi-6** Reliable networking in Cellular/Wireless |
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
curl -fsSL https://get.yomo.run | sh
```

Verify if the CLI was installed successfully

```bash
$ yomo version

YoMo CLI Version: v1.1.1
Runtime Version: v1.8.1
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
	"encoding/json"
	"fmt"
	"time"

	"github.com/yomorun/yomo/rx"
)

// NoiseData represents the structure of data
type NoiseData struct {
	Noise float32 `json:"noise"` // Noise value
	Time  int64   `json:"time"`  // Timestamp (ms)
	From  string  `json:"from"`  // Source IP
}

var echo = func(_ context.Context, i interface{}) (interface{}, error) {
	value := i.(*NoiseData)
	value.Noise = value.Noise / 10
	rightNow := time.Now().UnixNano() / int64(time.Millisecond)
	fmt.Println(fmt.Sprintf("[%s] %d > value: %f ⚡️=%dms", value.From, value.Time, value.Noise, rightNow-value.Time))
	return value.Noise, nil
}

// Handler will handle data in Rx way
func Handler(rxstream rx.Stream) rx.Stream {
	stream := rxstream.
		Unmarshal(json.Unmarshal, func() interface{} { return &NoiseData{} }).
		Debounce(50).
		Map(echo).
		StdOut()

	return stream
}

func DataTags() []byte {
	return []byte{0x33}
}

```

### 3. Build and run

1. Run `yomo dev` from the terminal. you will see the following message:

```sh
$ yomo dev

ℹ️  YoMo Stream Function file: app.go
⌛  Create YoMo Stream Function instance...
⌛  YoMo Stream Function building...
✅  Success! YoMo Stream Function build.
ℹ️   YoMo Stream Function is running...
2021/11/16 10:02:43 [core:client]  has connected to yomo-app-demo (dev.yomo.run:9140)
[localhost] 1637028164050 > value: 6.575044 ⚡️=9ms
[StdOut]:  6.5750437
[localhost] 1637028164151 > value: 10.076103 ⚡️=5ms
[StdOut]:  10.076103
[localhost] 1637028164251 > value: 15.560066 ⚡️=8ms
[StdOut]:  15.560066
[localhost] 1637028164352 > value: 15.330824 ⚡️=2ms
[StdOut]:  15.330824
[localhost] 1637028164453 > value: 10.859857 ⚡️=7ms
[StdOut]:  10.859857
```

Congratulations! You have done your first YoMo Stream Function.


## 🧩 Interop

### Metaverse Workplace (Virtual Office) with YoMo

+ [Frontend](https://github.com/yomorun/yomo-metaverse-workplace-nextjs)
+ [Backend](https://github.com/yomorun/yomo-vhq-backend)

### Sources

+ [Connect EMQ X Broker to YoMo](https://github.com/yomorun/yomo-source-emqx-starter)
+ [Connect MQTT to YoMo](https://github.com/yomorun/yomo-source-mqtt-broker-starter)

### Stream Functions

+ [Write a Stream Function with WebAssembly by WasmEdge](https://github.com/yomorun/yomo-wasmedge-tensorflow)

### Output Connectors

+ [Connect to FaunaDB to store post-processed result the serverless way](https://github.com/yomorun/yomo-sink-faunadb-example)
+ Connect to InfluxDB to store post-processed result
+ [Connect to TDEngine to store post-processed result](https://github.com/yomorun/yomo-sink-tdengine-example)

## 🗺 Location Insensitive Deployment

![yomo-flow-arch](https://docs.yomo.run/yomo-flow-arch.jpg)

## 📚 Documentation

+ `YoMo-Source`: [docs.yomo.run/source](https://docs.yomo.run/source)
+ `YoMo-Stream-Function`: [docs.yomo.run/stream-function](https://docs.yomo.run/stream-fn)
+ `YoMo-Zipper`: [docs.yomo.run/zipper](https://docs.yomo.run/zipper)
+ `Stream Processing in Rx way`: [Rx](https://docs.yomo.run/rx)
+ `Faster than real-time codec`: [Y3](https://github.com/yomorun/y3-codec)

[YoMo](https://yomo.run) ❤️ [Vercel](https://vercel.com/?utm_source=yomorun&utm_campaign=oss), our documentation website is

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

## 🏄‍♂️ Best Practice in Production

[Discussion #314](https://github.com/yomorun/yomo/discussions/314) Tips: YoMo/QUIC Server Performance Tuning 

## License

[Apache License 2.0](http://www.apache.org/licenses/LICENSE-2.0.html)
