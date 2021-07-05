# Multi stream functions example

This example represents how YoMo works with multiple stream functions.

## Code structure

+ `source`: Mocking data of a Noise Decibel Detection Sensor. [yomo.run/source](https://yomo.run/source)
+ `stream-fn-1` (formerly flow): Calculate the noise value in real-time. [yomo.run/stream-function](https://yomo.run/flow)
+ `stream-fn-2` (formerly flow): Print the warning message when the noise value reaches the threshold. [yomo.run/stream-function](https://yomo.run/flow)
+ `yomo-server` (formerly zipper): Orchestrate a workflow that receives the data from `source`, stream computing in `stream-fn-1` and `stream-fn-2` [yomo.run/yomo-server](https://yomo.run/zipper)

## How to run the example

### 1. Install YoMo CLI

Please visit [YoMo Getting Started](https://github.com/yomorun/yomo#1-install-cli) for details.

### 2. Run [yomo-server](https://yomo.run/zipper)

```bash
yomo serve -c ./yomo-server/workflow.yaml

ℹ️   Found 2 stream functions in yomo-server config
ℹ️   Stream Function 1: Noise-1
ℹ️   Stream Function 2: Noise-2
ℹ️   Running YoMo Server...
```

### 3. Run [stream-fn-1](https://yomo.run/flow)

```bash
yomo run ./stream-fn-1/app.go -n Noise-1

ℹ️  YoMo Stream Function file: example/multi-stream-fn/stream-fn-1/app.go
⌛  Create YoMo Stream Function instance...
ℹ️  Starting YoMo Stream Function instance with Name: Noise-1. Host: localhost. Port: 9000.
⌛  YoMo Stream Function building...
✅  Success! YoMo Stream Function build.
ℹ️  YoMo Stream Function is running...
2021/07/05 19:14:24 Connecting to yomo-server localhost:9000...
2021/07/05 19:14:24 ✅ Connected to yomo-server localhost:9000.
```

### 4. Run [stream-fn-2](https://yomo.run/flow)

```bash
yomo run ./stream-fn-2/app.go -n Noise-2

ℹ️  YoMo Stream Function file: example/multi-stream-fn/stream-fn-2/app.go
⌛  Create YoMo Stream Function instance...
ℹ️  Starting YoMo Stream Function instance with Name: Noise-2. Host: localhost. Port: 9000.
⌛  YoMo Stream Function building...
✅  Success! YoMo Stream Function build.
ℹ️  YoMo Stream Function is running...
2021/07/05 19:14:24 Connecting to yomo-server localhost:9000...
2021/07/05 19:14:24 ✅ Connected to yomo-server localhost:9000.
```

### 5. Run [yomo-source](https://yomo.run/source)

```bash
go run ./source/main.go

2021/07/05 19:15:00 Connecting to yomo-server localhost:9000 ...
2021/07/05 19:15:00 ✅ Connected to yomo-server localhost:9000
2021/07/05 19:15:00 ✅ Emit {157.14272 1621491060839 localhost} to yomo-server
2021/07/05 19:15:00 ✅ Emit {149.61421 1621491060942 localhost} to yomo-server
2021/07/05 19:15:00 ✅ Emit {187.12460 1621491061043 localhost} to yomo-server
2021/07/05 19:15:00 ✅ Emit {164.58117 1621491061146 localhost} to yomo-server
```

### Results

The terminal of `stream-fun-1` will print the real-time noise value.

```bash
[localhost] 1621491060839 > value: 15.714272 ⚡️=1ms
[localhost] 1621491060942 > value: 14.961421 ⚡️=1ms
[localhost] 1621491061043 > value: 18.712460 ⚡️=1ms
[localhost] 1621491061146 > value: 1.071311 ⚡️=1ms
[localhost] 1621491061246 > value: 16.458117 ⚡️=1ms
```

The terminal of `stream-fun-2` will show the warning when the value reaches the threshold.

```bash
receive noise value: 15.714272
receive noise value: 14.961421
receive noise value: 18.712460
❗ value: 18.712460 reaches the threshold 16! 𝚫=2.712460
[localhost] 1621491061146 > value: 1.071311 ⚡️=1ms
[localhost] 1621491061246 > value: 16.458117 ⚡️=1ms
❗ value: 16.458117 reaches the threshold 16! 𝚫=0.458117
```
