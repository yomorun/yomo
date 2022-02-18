# Multi stream functions example

This example represents how YoMo works with multiple stream functions.

## Code structure

+ `source`: Mocking data of a Sound Sensor. [docs.yomo.run/source](https://docs.yomo.run/source)
+ `stream-fn-1`: Calculate the sound value in real-time. [docs.yomo.run/stream-function](https://docs.yomo.run/stream-fn)
+ `stream-fn-2`: Print the warning message when the noise value reaches the threshold. [docs.yomo.run/stream-function](https://docs.yomo.run/stream-fn)
+ `stream-fn-3`: The raw stream is immutable, `stream-fn-3` can still observe the raw data and calculate the average value in a sliding window. [docs.yomo.run/stream-function](https://docs.yomo.run/stream-fn)
+ `zipper`: Orchestrate a workflow that receives the data from `source`, stream computing in `stream-fn-1`, `stream-fn-2` and `stream-fn-3` [docs.yomo.run/zipper](https://docs.yomo.run/zipper)

## How to run the example

### 1. Install YoMo CLI

### Binary (Recommended)

```bash
$ curl -fsSL "https://bina.egoist.sh/yomorun/cli?name=yomo" | sh

  ==> Resolved version latest to v0.1.7
  ==> Downloading asset for darwin amd64
  ==> Installing yomo to /usr/local/bin
  ==> Installation complete
```

### Or build from source

```bash
$ go install github.com/yomorun/cli/yomo@latest
$ yomo version
YoMo CLI Version: v0.1.7
```

> You could install [task](https://taskfile.dev/#/installation) and run the following steps in one command `task example:multi-stream-fn`.

### 2. Run [YoMo-Zipper](https://docs.yomo.run/zipper)

```bash
yomo serve -c ./zipper/workflow.yaml

ℹ️   Found 3 stream functions in YoMo-Zipper config
ℹ️   Stream Function 1: Noise-1
ℹ️   Stream Function 2: Noise-2
ℹ️   Stream Function 3: Noise-3
ℹ️   Running YoMo Zipper...
```

### 3. Run [stream-fn-1](https://docs.yomo.run/stream-fn)

```bash
go run ./stream-fn-1/app.go

2021/07/05 19:14:24 [core:client] use credential: [None]
2021/07/05 19:14:24 [core:client] ❤️  [Noise-1]([::]:64869) is connected to YoMo-Zipper localhost:9000
```

### 4. Run [stream-fn-2](https://docs.yomo.run/stream-fn)

```bash
go run ./stream-fn-2/app.go

2021/07/05 19:14:24 [core:client] use credential: [None]
2021/07/05 19:14:24 [core:client] ❤️  [Noise-2]([::]:55565) is connected to YoMo-Zipper localhost:9000
```

### 5. Run [stream-fn-3](https://docs.yomo.run/stream-fn)

```bash
go run ./stream-fn-3/app.go

2021/07/05 19:14:24 [core:client] use credential: [None]
2021/07/05 19:14:24 [core:client] ❤️  [Noise-3]([::]:50019) is connected to YoMo-Zipper localhost:9000
```

### 6. Run [yomo-source](https://docs.yomo.run/source)

```bash
go run ./source/main.go

2021/07/05 19:15:00 Connecting to YoMo-Zipper localhost:9000 ...
2021/07/05 19:15:00 ✅ Connected to YoMo-Zipper localhost:9000
2021/07/05 19:15:00 ✅ Emit {157.14272 1621491060839 localhost} to YoMo-Zipper
2021/07/05 19:15:00 ✅ Emit {149.61421 1621491060942 localhost} to YoMo-Zipper
2021/07/05 19:15:00 ✅ Emit {187.12460 1621491061043 localhost} to YoMo-Zipper
2021/07/05 19:15:00 ✅ Emit {164.58117 1621491061146 localhost} to YoMo-Zipper
```

### Results

The terminal of `stream-fn-1` will print the real-time noise value.

```bash
[localhost] 1621491060839 > value: 15.714272 ⚡️=1ms
[localhost] 1621491060942 > value: 14.961421 ⚡️=1ms
[localhost] 1621491061043 > value: 18.712460 ⚡️=1ms
[localhost] 1621491061146 > value: 1.071311 ⚡️=1ms
[localhost] 1621491061246 > value: 16.458117 ⚡️=1ms
```

The terminal of `stream-fn-2` will show the warning when the value reaches the threshold.

```bash
receive noise value: 15.714272
receive noise value: 14.961421
receive noise value: 18.712460
❗ value: 18.712460 reaches the threshold 16! 𝚫=2.712460
[localhost] 1621491061146 > value: 1.071311 ⚡️=1ms
[localhost] 1621491061246 > value: 16.458117 ⚡️=1ms
❗ value: 16.458117 reaches the threshold 16! 𝚫=0.458117
```

The terminal of `stream-fn-3` will show the average value in a sliding window 10s.

```bash
[StdOut]:  15.714272
[StdOut]:  14.961421
[StdOut]:  18.712460
[StdOut]:  1.071311
[StdOut]:  16.458117
🧩 average value in last 10000 ms: 10.931099!
```
