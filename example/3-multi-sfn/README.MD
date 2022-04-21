# Multi stream functions example

This example represents how YoMo works with multiple stream functions. The last cascading function is being listened by 2 stream functions wheras instead of sending only to one randomly, we would broadcast to all those subscribed on the function.

## Code structure

- `source`: Mocking data of a Sound Sensor. [docs.yomo.run/source](https://docs.yomo.run/source)
- `stream-fn-1`: Calculate the sound value in real-time. [docs.yomo.run/stream-function](https://docs.yomo.run/stream-fn)
- `stream-fn-2`: Print the warning message when the noise value reaches the threshold. [docs.yomo.run/stream-function](https://docs.yomo.run/stream-fn)
- `stream-fn-3a`: The raw stream is immutable, `stream-fn-3a` can still observe the raw data and calculate the average value in a sliding window. [docs.yomo.run/stream-function](https://docs.yomo.run/stream-fn)
- `stream-fn-3b`: The raw stream is immutable, `stream-fn-3b` can still observe the raw data and calculate the average value in a sliding window. [docs.yomo.run/stream-function](https://docs.yomo.run/stream-fn)
- `zipper`: Orchestrate a workflow that receives the data from `source`, stream computing in `stream-fn-1`, `stream-fn-2` and `stream-fn-3` [docs.yomo.run/zipper](https://docs.yomo.run/zipper)

## Prepare

Install YoMo CLI

### Binary (Recommended)

```bash
$ curl -fsSL "https://bina.egoist.sh/yomorun/cli?name=yomo" | sh

  ==> Resolved version latest to v1.0.0
  ==> Downloading asset for darwin amd64
  ==> Installing yomo to /usr/local/bin
  ==> Installation complete
```

### Or build from source

```bash
$ go install github.com/yomorun/cli/yomo@latest
$ yomo version
YoMo CLI Version: v1.0.0
```

## Option 1: Auto Run

`task run`

```bash
$ task run

task: [zipper] go run zipper/main.go
task: [source-build] go build -o ./bin/source source/main.go
task: [sfn-1] go build -o ./bin/sfn-1 ./stream-fn-1/app.go
task: [sfn-2] go build -o ./bin/sfn-2 ./stream-fn-2/app.go
task: [sfn-3a] go build -o ./bin/sfn-3a ./stream-fn-3a/app.go
task: [sfn-3b] go build -o ./bin/sfn-3b ./stream-fn-3b/app.go
task: [sfn-1] ./bin/sfn-1
task: [sfn-3a] ./bin/sfn-3a
task: [sfn-3b] ./bin/sfn-3b
task: [source] go build -o ./bin/source source/main.go
task: [sfn-2] ./bin/sfn-2
[zipper] 2022-04-20 21:48:31.754        [yomo:zipper] Listening SIGUSR1, SIGUSR2, SIGTERM/SIGINT...
[zipper] 2022-04-20 21:48:31.757        [core:server] ✅ [Zipper] Listening on: 127.0.0.1:9000, MODE: DEVELOPMENT, QUIC: [v1 draft-29], AUTH: [None]
task: [source] ./bin/source
[sfn-1] 2022-04-20 21:48:31.961 [core:client] use credential: [None]
[sfn-1] 2022-04-20 21:48:31.969 [core:client] ❤️  [Noise-1]([::]:52350) is connected to YoMo-Zipper localhost:9000
[zipper] 2022-04-20 21:48:31.969        [core:server] ❤️  <Stream Function> [::Noise-1](127.0.0.1:52350) is connected!
[sfn-3b] 2022-04-20 21:48:32.043        [core:client] use credential: [None]
[sfn-3b] 2022-04-20 21:48:32.049        [core:client] ❤️  [Noise-3]([::]:64524) is connected to YoMo-Zipper localhost:9000
[zipper] 2022-04-20 21:48:32.050        [core:server] ❤️  <Stream Function> [::Noise-3](127.0.0.1:64524) is connected!
[sfn-3a] 2022-04-20 21:48:32.134        [core:client] use credential: [None]
[sfn-3a] 2022-04-20 21:48:32.141        [core:client] ❤️  [Noise-3]([::]:58187) is connected to YoMo-Zipper localhost:9000
[zipper] 2022-04-20 21:48:32.141        [core:server] ❤️  <Stream Function> [::Noise-3](127.0.0.1:58187) is connected!
[sfn-2] 2022-04-20 21:48:32.218 [core:client] use credential: [None]
[sfn-2] 2022-04-20 21:48:32.225 [core:client] ❤️  [Noise-2]([::]:63430) is connected to YoMo-Zipper localhost:9000
[zipper] 2022-04-20 21:48:32.225        [core:server] ❤️  <Stream Function> [::Noise-2](127.0.0.1:63430) is connected!
[source] 2022-04-20 21:48:32.314        [core:client] use credential: [None]
[source] 2022-04-20 21:48:32.321        [core:client] ❤️  [yomo-source]([::]:63332) is connected to YoMo-Zipper localhost:9000
[source] 2022-04-20 21:48:32.322        ✅ Emit {194.21623 1650505712321 localhost} to YoMo-Zipper
[zipper] 2022-04-20 21:48:32.322        [core:server] ❤️  <Source> [::yomo-source](127.0.0.1:63332) is connected!
[sfn-1] 2022-04-20 21:48:32.323 ✅ [localhost] 1650505712321 > value: 19.421623 ⚡️=2ms
[sfn-2] ✅ receive noise value: 19.421623
[sfn-3b] 2022-04-20 21:48:32.324        ✅ [fn3] observe <- 19.421623
[sfn-3a] 2022-04-20 21:48:32.324        ✅ [fn3] observe <- 19.421623
[source] 2022-04-20 21:48:32.823        ✅ Emit {87.032425 1650505712823 localhost} to YoMo-Zipper
[sfn-1] 2022-04-20 21:48:32.825 ✅ [localhost] 1650505712823 > value: 8.703242 ⚡️=2ms
[sfn-2] ❗ value: 19.421623 reaches the threshold 16! 𝚫=3.421623✅ receive noise value: 8.703242
[sfn-3a] 2022-04-20 21:48:32.826        ✅ [fn3] observe <- 8.703242
[sfn-3b] 2022-04-20 21:48:32.826        ✅ [fn3] observe <- 8.703242
[sfn-3b] 2022-04-20 21:48:33.050        🧩 average value in last 10000 ms: 19.421623!
[sfn-3b] 2022-04-20 21:48:33.050        ❗❗  average value in last 10000 ms: 19.421623 reaches the threshold 13!
[sfn-3a] 2022-04-20 21:48:33.141        🧩 average value in last 10000 ms: 19.421623!
[sfn-3a] 2022-04-20 21:48:33.141        ❗❗  average value in last 10000 ms: 19.421623 reaches the threshold 13!
```

## Option 2: Manual

### Run [YoMo-Zipper](https://docs.yomo.run/zipper)

```bash
yomo serve -c ./zipper/workflow.yaml

ℹ️   Found 3 stream functions in YoMo-Zipper config
ℹ️   Stream Function 1: Noise-1
ℹ️   Stream Function 2: Noise-2
ℹ️   Stream Function 3: Noise-3
ℹ️   Running YoMo Zipper...
```

### Run [stream-fn-1](https://docs.yomo.run/stream-fn)

```bash
go run ./stream-fn-1/app.go

2021/07/05 19:14:24 [core:client] use credential: [None]
2021/07/05 19:14:24 [core:client] ❤️  [Noise-1]([::]:64869) is connected to YoMo-Zipper localhost:9000
```

### Run [stream-fn-2](https://docs.yomo.run/stream-fn)

```bash
go run ./stream-fn-2/app.go

2021/07/05 19:14:24 [core:client] use credential: [None]
2021/07/05 19:14:24 [core:client] ❤️  [Noise-2]([::]:55565) is connected to YoMo-Zipper localhost:9000
```

### Run [stream-fn-3](https://docs.yomo.run/stream-fn)

```bash
go run ./stream-fn-3/app.go

2021/07/05 19:14:24 [core:client] use credential: [None]
2021/07/05 19:14:24 [core:client] ❤️  [Noise-3]([::]:50019) is connected to YoMo-Zipper localhost:9000
```

### Run [yomo-source](https://docs.yomo.run/source)

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

The terminal of `stream-fn-3a` will show the average value in a sliding window 10s.

```bash
[StdOut]:  15.714272
[StdOut]:  14.961421
[StdOut]:  18.712460
[StdOut]:  1.071311
[StdOut]:  16.458117
🧩 average value in last 10000 ms: 10.931099!
```

The terminal of `stream-fn-3b` will show the average value in a sliding window 10s.

```bash
[StdOut]:  15.714272
[StdOut]:  14.961421
[StdOut]:  18.712460
[StdOut]:  1.071311
[StdOut]:  16.458117
🧩 average value in last 10000 ms: 10.931099!
```
