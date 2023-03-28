<p align="center">
  <img width="200px" height="200px" src="https://blog.yomo.run/static/images/logo.png" />
</p>

# YoMo ![Go](https://github.com/yomorun/yomo/workflows/Go/badge.svg) [![codecov](https://codecov.io/gh/yomorun/yomo/branch/master/graph/badge.svg?token=MHCE5TZWKM)](https://codecov.io/gh/yomorun/yomo) [![Discord](https://img.shields.io/discord/770589787404369930.svg?label=discord&logo=discord&logoColor=ffffff&color=7389D8&labelColor=6A7EC2)](https://discord.gg/RMtNhx7vds)

YoMo is an open-source Streaming Serverless Framework for building Low-latency Edge Computing applications. Built atop QUIC Transport Protocol and Functional Reactive Programming interface, it makes real-time data processing reliable, secure, and easy.

Official Website: 🦖[https://yomo.run](https://yomo.run)

💚 We care about: **The Demand For Real-Time Digital User Experiences**

It’s no secret that today’s users want instant gratification, every productivity application is more powerful when it's collaborative. But, currently, when we talk about `distribution`, it represents **distribution in data center**. API is far away from their users from all over the world.

If an application can be deployed anywhere close to their end users, solve the problem, this is **Geo-distributed System Architecture**:

<img width="580" alt="yomo geo-distributed system" src="https://user-images.githubusercontent.com/65603/162367572-5a0417fa-e2b2-4d35-8c92-2c95d461706d.png">

## 🌶 Features

|     | **Features**                                                                                                 |
| --- | ------------------------------------------------------------------------------------------------------------ |
| ⚡️   | **Low-latency** Guaranteed by implementing atop QUIC [QUIC](https://datatracker.ietf.org/wg/quic/documents/) |
| 🔐   | **Security** TLS v1.3 on every data packet by design                                                         |
| 📱   | **5G/WiFi-6** Reliable networking in Cellular/Wireless                                                       |
| 🌎   | **Geo-Distributed Edge Mesh** Edge-Mesh Native architecture makes your services close to end users           |
| 📸   | **Event-First** Architecture leverages serverless service to be event driven and elastic                     |
| 🦖   | **Streaming Serverless** Write only a few lines of code to build applications and microservices              |
| 🚀   | **Y3** a [faster than real-time codec](https://github.com/yomorun/y3-codec-golang)                           |
| 📨   | **Reactive** stream processing based on [Rx](http://reactivex.io/documentation/operators.html)               |

## 🚀 Getting Started

### Prerequisite

[Install Go](https://golang.org/doc/install)

### 1. Install CLI

```bash
curl -fsSL https://get.yomo.run | sh
```

Verify if the CLI was installed successfully

```bash
yomo version
```

### 2. Write your first stream function with Rust and WebAssembly

In this demo, we will create a rust project observing a data stream and uppercase every string received, 
then create send them to next process unit with another data stream:

```rust
#[yomo::init]
fn init() -> anyhow::Result<Vec<u32>> {
    // return observe datatags
    Ok(vec![0x33])
}

#[yomo::handler]
fn handler(input: &[u8]) -> anyhow::Result<(u32, Vec<u8>)> {
    println!("wasm rust sfn received {} bytes", input.len());

    // parse input from bytes
    let input = String::from_utf8(input.to_vec())?;

    // your app logic goes here
    let output = input.to_uppercase();

    // return the datatag and output bytes
    Ok((0x34, output.into_bytes()))
}
```

The full project can be found at [example/7-wasm/rust](example/7-wasm/rust).

### 3. Build and run

Let's start the `YoMo Zipper` service, which is a service that processes data from the `YoMo Source` and coordinates the `YoMo Stream Function` to process a specific data stream:

```bash
yomo serve -c example/uppercase/workflow.yaml
```

Then, start the WebAssembly Stream Function, get ready to process data:

```bash
cd example/7-wasm/sfn/rust
rustup target add wasm32-wasi
cargo build --release --target wasm32-wasi
YOMO_SFN_NAME=upper bin/yomo run example/7-wasm/sfn/rust/target/wasm32-wasi/release/sfn.wasm
```

Finally, let's try send data:

```bash
$ go run example/uppercase/source/main.go

time=2023-03-28T09:41:13.782+08:00 level=INFO msg="use credential" component=client client_type=Source client_id=9bqB-J8I3l-6YHkuhB11I client_name=source credential_name=none
time=2023-03-28T09:41:13.786+08:00 level=INFO msg="use credential" component=client client_type="Stream Function" client_id=jS-GCGUBMRW1yTnyU6Yke client_name=sink credential_name=none
2023/03/28 09:41:13 [send] [0] Hello, YoMo!
2023/03/28 09:41:13 [recv] [0] HELLO, YOMO!
2023/03/28 09:41:14 [send] [1] Hello, YoMo!
2023/03/28 09:41:14 [recv] [1] HELLO, YOMO!
2023/03/28 09:41:15 [send] [2] Hello, YoMo!
2023/03/28 09:41:15 [recv] [2] HELLO, YOMO!
2023/03/28 09:41:16 [send] [3] Hello, YoMo!
2023/03/28 09:41:16 [recv] [3] HELLO, YOMO!
2023/03/28 09:41:17 [send] [4] Hello, YoMo!
2023/03/28 09:41:17 [recv] [4] HELLO, YOMO!
2023/03/28 09:41:18 [send] [5] Hello, YoMo!
2023/03/28 09:41:18 [recv] [5] HELLO, YOMO!
```

It works!

There are many other examples that can help reduce the learning curve:

- [0-basic](./example/0-basic/): Write Stream Function in pure golang.
- [1-pipeline](./example/1-pipeline/): Unix Pipeline over Cloud.
- [2-iopipe](./example/2-iopipe/): Unix Pipeline over Cloud.
- [3-multi-sfn](./example/3-multi-sfn/): Write programs that do one thing and do it well. Write programs to work together. -- [Doug Mcllroy](https://en.wikipedia.org/wiki/Unix_philosophy)
- [4-cascading-zipper](./example/4-cascading-zipper/): Flexible adjustment of sfn deployment and run locations.
- [5-backflow](./example/5-backflow/)
- [6-mesh](./example/6-mesh/): Demonstrate how to put your serverless closer to end-user.
- [7-wasm](./example/7-wasm/): Implement Stream Function by WebAssembly in `c`, `go`, `rust` and even [zig](https://ziglang.org).
- [8-deno](./example/8-deno/): Demonstrate how to write Stream Function with TypeScript and [deno](https://deno.com).
- [9-cli](./example/9-cli/): Implement Stream Function in [Rx](https://reactivex.io/) way.

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
