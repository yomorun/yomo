# Basic example

This example represents how YoMo works with the mock data of sound sensor.

## Code structure

+ `source`: sending mock data of sound sensor [yomo.run/source](https://yomo.run/source)
+ `flow`: calculate the real-time noise level and print the warning message when it reaches the threshold. [yomo.run/flow](https://yomo.run/flow)
+ `sink`: mock persisting the real-time noise level in DB. [yomo.run/sink](https://yomo.run/sink)
+ `zipper`: orchestrate a workflow that receives the data from `source`, stream computing in `flow` and output the result to `sink` [yomo.run/zipper](https://yomo.run/zipper)

## How to run the example

### 1. Install YoMo CLI

Please visit [YoMo Getting Started](https://github.com/yomorun/yomo#1-install-cli) for details.

### 2. Run [yomo-zipper](https://yomo.run/zipper)

```bash
yomo wf run ./zipper/workflow.yaml

2021/05/20 14:09:42 Found 1 flows in zipper config
2021/05/20 14:09:42 Flow 1: Noise
2021/05/20 14:09:42 Found 1 sinks in zipper config
2021/05/20 14:09:42 Sink 1: MockDB
2021/05/20 14:09:42 Running YoMo workflow...
2021/05/20 14:09:42 ✅ Listening on 0.0.0.0:9000
```

### 3. Run [yomo-flow](https://yomo.run/flow)

```bash
yomo run ./flow/app.go -n Noise

2021/05/20 14:10:15 Building the Serverless Function File...
2021/05/20 14:10:17 Connecting to zipper localhost:9000 ...
2021/05/20 14:10:17 ✅ Connected to zipper localhost:9000
2021/05/20 14:10:17 Running the Serverless Function.
```

### 4. Run [yomo-sink](https://yomo.run/sink)

```bash
yomo run ./sink/app.go -n MockDB

2021/05/20 14:10:28 Building the Serverless Function File...
2021/05/20 14:10:29 Connecting to zipper localhost:9000 ...
2021/05/20 14:10:29 ✅ Connected to zipper localhost:9000
2021/05/20 14:10:29 Running the Serverless Function.
```

### 5. Run [yomo-source](https://yomo.run/source)

```bash
go run ./source/main.go

2021/05/20 14:10:28 Building the Serverless Function File...
2021/05/20 14:11:00 Connecting to zipper localhost:9000 ...
2021/05/20 14:11:00 ✅ Connected to zipper localhost:9000
2021/05/20 14:11:00 ✅ Emit {99.11785 1621491060031 localhost} to yomo-zipper
2021/05/20 14:11:00 ✅ Emit {145.5075 1621491060131 localhost} to yomo-zipper
2021/05/20 14:11:00 ✅ Emit {118.27067 1621491060233 localhost} to yomo-zipper
2021/05/20 14:11:00 ✅ Emit {56.369446 1621491060335 localhost} to yomo-zipper
```

### Results

#### yomo-flow

The terminal of `yomo-flow` will print the real-time noise level, and show the waring when noise level reaches the threshold.

```bash
[localhost] 1621491060839 > value: 15.714272 ⚡️=1ms
[localhost] 1621491060942 > value: 14.961421 ⚡️=1ms
[localhost] 1621491061043 > value: 18.712460 ⚡️=1ms
❗ value: 18.712460 reaches the threshold 16! 𝚫=2.712460
[localhost] 1621491061146 > value: 1.071311 ⚡️=1ms
[localhost] 1621491061246 > value: 16.458117 ⚡️=1ms
❗ value: 16.458117 reaches the threshold 16! 𝚫=0.458117
🧩 average value in last 10000 ms: 10.918112!
```

#### yomo-sink

The terminal of `yomo-sink` will print the message for saving the data in DB.

```bash
save `18.71246` to FaunaDB
save `1.0713108` to FaunaDB
save `16.458117` to FaunaDB
save `12.397432` to FaunaDB
save `15.227814` to FaunaDB
save `14.787642` to FaunaDB
save `17.85902` to FaunaDB
```
