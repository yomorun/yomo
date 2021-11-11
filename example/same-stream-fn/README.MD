# Multi stream functions example

This example represents how YoMo works with multiple instances of the same stream function, `zipper` will send the data to these instances alternately by round-robin.

## Code structure

+ `source`: Mocking data of a Noise Decibel Detection Sensor. [yomo.run/source](https://docs.yomo.run/source)
+ `stream-fn` (formerly flow): Calculate the noise value in real-time. [yomo.run/stream-function](https://docs.yomo.run/stream-function)
+ `zipper`: Orchestrate a workflow that receives the data from `source`, stream computing in `stream-fn-1`, `stream-fn-2` and `stream-fn-3` [yomo.run/zipper](https://docs.yomo.run/zipper)

## How to run the example

### 1. Install YoMo CLI

Please visit [YoMo Getting Started](https://github.com/yomorun/yomo#1-install-cli) for details.

### 2. Run [YoMo-Zipper](https://docs.yomo.run/zipper)

```bash
yomo serve -c ./zipper/workflow.yaml

2021/11/11 16:39:34 [yomo:zipper] [AddWorkflow] 0, Noise
ℹ️   Running YoMo-Zipper...
2021/11/11 16:39:34 [yomo:zipper] Listening SIGTERM/SIGINT...
2021/11/11 16:39:34 [core:server] ✅ (name:Service) Listening on: 127.0.0.1:9000, QUIC: [v1 draft-29]
```

### 3. Run first [stream-fn-1](https://docs.yomo.run/stream-function) instance

```bash
go run ./stream-fn/app.go

2021/11/11 16:41:02 [core:client] use credential: [None]
2021/11/11 16:41:02 handshake frame=&frame.HandshakeFrame{Name:"Noise", ClientType:0x5d, authType:0x0, authPayload:[]uint8(nil)},err=<nil>
2021/11/11 16:41:02 [core:client] ❤️  [Noise] is connected to YoMo-Zipper localhost:9000
```

### 4. Run second [stream-fn-2](https://docs.yomo.run/stream-function) instance

```bash
go run ./stream-fn/app.go

2021/11/11 16:41:48 [core:client] use credential: [None]
2021/11/11 16:41:48 handshake frame=&frame.HandshakeFrame{Name:"Noise", ClientType:0x5d, authType:0x0, authPayload:[]uint8(nil)},err=<nil>
2021/11/11 16:41:48 [core:client] ❤️  [Noise] is connected to YoMo-Zipper localhost:9000
```

### 5. Run [yomo-source](https://docs.yomo.run/source)

```bash
go run ./source/main.go

2021/11/11 16:42:32 [core:client] use credential: [None]
2021/11/11 16:42:32 handshake frame=&frame.HandshakeFrame{Name:"yomo-source", ClientType:0x5f, authType:0x0, authPayload:[]uint8(nil)},err=<nil>
2021/11/11 16:42:32 [core:client] ❤️  [yomo-source] is connected to YoMo-Zipper localhost:9000
2021/11/11 16:42:32 ✅ Emit {139.61163 1636620152801 localhost} to YoMo-Zipper
2021/11/11 16:42:33 ✅ Emit {124.2831 1636620153104 localhost} to YoMo-Zipper
2021/11/11 16:42:33 ✅ Emit {108.49202 1636620153409 localhost} to YoMo-Zipper
2021/11/11 16:42:33 ✅ Emit {87.19999 1636620153709 localhost} to YoMo-Zipper
```

### Results

The two `stream-fn` will receive the data alternately by round-robin.

The terminal of `stream-fn-1` will print:

```bash
2021/11/11 16:42:33 [localhost] 1636620153104 > value: 12.428309 ⚡️=1ms
2021/11/11 16:42:33 [localhost] 1636620153709 > value: 8.719999 ⚡️=2ms
```

The terminal of `stream-fn-2` will print:

```bash
2021/11/11 16:42:32 [localhost] 1636620152801 > value: 13.961164 ⚡️=1ms
2021/11/11 16:42:33 [localhost] 1636620153409 > value: 10.849202 ⚡️=0ms
```
