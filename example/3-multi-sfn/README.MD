# Multi stream functions example

This example represents how YoMo works with multiple stream functions.

## Code structure

- `source`: Mocking data of a Sound Sensor. [docs.yomo.run/source](https://yomo.run/docs/api/source)
- `stream-fn-1`: Calculate the sound value in real-time. [docs.yomo.run/stream-function](https://yomo.run/docs/api/sfn)
- `stream-fn-2`: Print the warning message when the noise value reaches the threshold. [docs.yomo.run/stream-function](https://yomo.run/docs/api/sfn)
- `stream-fn-3`: The raw stream is immutable, `stream-fn-3` can still observe the raw data and calculate the average value in a sliding window. [docs.yomo.run/stream-function](https://yomo.run/docs/api/sfn)
- `zipper`: Orchestrate a workflow that receives the data from `source`, stream computing in `stream-fn-1`, `stream-fn-2` and `stream-fn-3` [docs.yomo.run/zipper](https://yomo.run/docs/cli/zipper)

## Prepare

Install YoMo CLI

### Binary (Recommended)

```bash
$ curl -fsSL https://get.yomo.run | sh

  ==> Resolved version latest to v1.0.0
  ==> Downloading asset for darwin amd64
  ==> Installing yomo to /usr/local/bin
  ==> Installation complete
```

### Or build from source

```bash
$ go install github.com/yomorun/yomo/cmd/yomo@latest
$ yomo version
YoMo CLI Version: v1.0.0
```

## Option 1: Auto Run

`task run`

```bash
$ task run

task: [zipper] yomo serve -c ./zipper/config.yaml
task: [sfn-3] go build -o ./bin/sfn-3 ./stream-fn-3/app.go
task: [source-build] go build -o ./bin/source source/main.go
task: [sfn-1] go build -o ./bin/sfn-1 ./stream-fn-1/app.go
task: [sfn-2] go build -o ./bin/sfn-2 ./stream-fn-2/app.go
[zipper] Using config file: ./zipper/config.yaml
[zipper] ℹ️   Running YoMo-Zipper...
[zipper] 2022-02-20 17:10:03.764	[yomo:zipper] Listening SIGUSR1, SIGUSR2, SIGTERM/SIGINT...
[zipper] 2022-02-20 17:10:03.773	[core:server] ✅ [Service] Listening on: 127.0.0.1:9000, QUIC: [v1 draft-29], AUTH: [None]
task: [sfn-1] ./bin/sfn-1
task: [sfn-2] ./bin/sfn-2
task: [sfn-3] ./bin/sfn-3
[sfn-3] 2022-02-20 17:10:04.121	[core:client] use credential: [None]
[sfn-1] 2022-02-20 17:10:04.122	[core:client] use credential: [None]
[sfn-2] 2022-02-20 17:10:04.122	[core:client] use credential: [None]
[sfn-3] 2022-02-20 17:10:04.124	[core:client] ❤️  [Noise-3]([::]:60308) is connected to YoMo-Zipper localhost:9000
[sfn-2] 2022-02-20 17:10:04.124	[core:client] ❤️  [Noise-2]([::]:59332) is connected to YoMo-Zipper localhost:9000
[sfn-1] 2022-02-20 17:10:04.125	[core:client] ❤️  [Noise-1]([::]:53199) is connected to YoMo-Zipper localhost:9000
[zipper] 2022-02-20 17:10:04.125	[core:server] ❤️  <Stream Function> [::Noise-2](127.0.0.1:59332) is connected!
[zipper] 2022-02-20 17:10:04.125	[core:server] ❤️  <Stream Function> [::Noise-3](127.0.0.1:60308) is connected!
[zipper] 2022-02-20 17:10:04.125	[core:server] ❤️  <Stream Function> [::Noise-1](127.0.0.1:53199) is connected!
task: [source] go build -o ./bin/source source/main.go
task: [source] ./bin/source
[source] 2022-02-20 17:10:05.032	[core:client] use credential: [None]
[source] 2022-02-20 17:10:05.038	[core:client] ❤️  [yomo-source]([::]:64310) is connected to YoMo-Zipper localhost:9000
[source] 2022-02-20 17:10:05.038	✅ Emit {121.417854 1645348205038 localhost} to YoMo-Zipper
[zipper] 2022-02-20 17:10:05.038	[core:server] ❤️  <Source> [::yomo-source](127.0.0.1:64310) is connected!
[sfn-1] 2022-02-20 17:10:05.039	✅ [localhost] 1645348205038 > value: 12.141786 ⚡️=1ms
[sfn-2] ✅ receive noise value: 12.141786
[sfn-3] 2022-02-20 17:10:05.040	✅ [fn3] observe <- 12.141786
[sfn-3] 2022-02-20 17:10:05.125	🧩 average value in last 10000 ms: 12.141786!
[sfn-3] 2022-02-20 17:10:06.125	🧩 average value in last 10000 ms: 12.141786!
[sfn-3] 2022-02-20 17:10:07.126	🧩 average value in last 10000 ms: 12.141786!
[sfn-3] 2022-02-20 17:10:08.127	🧩 average value in last 10000 ms: 12.141786!
[sfn-3] 2022-02-20 17:10:09.127	🧩 average value in last 10000 ms: 12.141786!
[source] 2022-02-20 17:10:10.039	✅ Emit {87.23943 1645348210039 localhost} to YoMo-Zipper
[sfn-1] 2022-02-20 17:10:10.041	✅ [localhost] 1645348210039 > value: 8.723944 ⚡️=2ms
[sfn-2] ✅ receive noise value: 8.723944
[sfn-3] 2022-02-20 17:10:10.045	✅ [fn3] observe <- 8.723944
[sfn-3] 2022-02-20 17:10:10.128	🧩 average value in last 10000 ms: 12.141786!
[sfn-3] 2022-02-20 17:10:11.128	🧩 average value in last 10000 ms: 12.141786!
```

## Option 2: Manual

### Run [YoMo-Zipper](https://yomo.run/docs/cli/zipper)

```bash
yomo serve -c ./zipper/config.yaml

ℹ️   Found 3 stream functions in YoMo-Zipper config
ℹ️   Stream Function 1: Noise-1
ℹ️   Stream Function 2: Noise-2
ℹ️   Stream Function 3: Noise-3
ℹ️   Running YoMo Zipper...
```

### Run [stream-fn-1](https://yomo.run/docs/api/sfn)

```bash
go run ./stream-fn-1/app.go

2021/07/05 19:14:24 [core:client] use credential: [None]
2021/07/05 19:14:24 [core:client] ❤️  [Noise-1]([::]:64869) is connected to YoMo-Zipper localhost:9000
```

### Run [stream-fn-2](https://yomo.run/docs/api/sfn)

```bash
go run ./stream-fn-2/app.go

2021/07/05 19:14:24 [core:client] use credential: [None]
2021/07/05 19:14:24 [core:client] ❤️  [Noise-2]([::]:55565) is connected to YoMo-Zipper localhost:9000
```

### Run [stream-fn-3](https://yomo.run/docs/api/sfn)

```bash
go run ./stream-fn-3/app.go

2021/07/05 19:14:24 [core:client] use credential: [None]
2021/07/05 19:14:24 [core:client] ❤️  [Noise-3]([::]:50019) is connected to YoMo-Zipper localhost:9000
```

### Run [yomo-source](https://yomo.run/docs/api/source)

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
