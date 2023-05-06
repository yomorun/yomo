# Cascading Zippers

This example represents how YoMo works with cascading zippers in mesh network.

## Code structure

- `source`: Mocking random data and send it to `zipper-1`. [docs.yomo.run/source](https://docs.yomo.run/source)
- `zipper-1`: Receive the streams from `source`, and broadcast it to downstream `zipper-2` in another region. [docs.yomo.run/zipper](https://docs.yomo.run/zipper)
- `zipper-2`: Receive the streams from upstream `zipper-1`. [docs.yomo.run/zipper](https://docs.yomo.run/zipper)
- `sfn`: Receive the streams from `zipper-2` and print it in terminal. [docs.yomo.run/stream-function](https://docs.yomo.run/stream-fn)

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
task: [zipper-2] yomo serve -c zipper_2.yaml
task: [zipper-1] yomo serve -c zipper_1.yaml
task: [source-build] go build -o ./bin/source source/source.go
task: [sfn-build] go build -o ./bin/sfn sfn/sfn_echo.go
[zipper-2] ℹ️   Running YoMo-Zipper...
[zipper-1] ℹ️   Running YoMo-Zipper...
task: [source] ./bin/source
task: [sfn] ./bin/sfn
[source] 2023/05/06 12:38:34 [source] ✅ Emit 3058996128 to YoMo-Zipper
[source] 2023/05/06 12:38:35 [source] ✅ Emit 970774474 to YoMo-Zipper
[sfn] 2023/05/06 12:38:35 >> [sfn] got tag=0x33, data=970774474
[source] 2023/05/06 12:38:36 [source] ✅ Emit 2422839449 to YoMo-Zipper
[sfn] 2023/05/06 12:38:36 >> [sfn] got tag=0x33, data=2422839449
[source] 2023/05/06 12:38:37 [source] ✅ Emit 1599851864 to YoMo-Zipper
[sfn] 2023/05/06 12:38:37 >> [sfn] got tag=0x33, data=1599851864
[source] 2023/05/06 12:38:38 [source] ✅ Emit 3745279519 to YoMo-Zipper
[sfn] 2023/05/06 12:38:38 >> [sfn] got tag=0x33, data=3745279519
[source] 2023/05/06 12:38:39 [source] ✅ Emit 1411262925 to YoMo-Zipper
[sfn] 2023/05/06 12:38:39 >> [sfn] got tag=0x33, data=1411262925
```

## Option 2: Manual

### Run [zipper-1](https://docs.yomo.run/zipper)

```bash
yomo serve -c zipper_1.yaml

ℹ️   Running YoMo-Zipper...
2021/11/11 16:09:54 [yomo:zipper] Listening SIGUSR1, SIGUSR2, SIGTERM/SIGINT..
2021/11/11 16:09:54 [core:server] ✅ [Zipper-1] Listening on: [::]:9001, QUIC: [v1 draft-29], AUTH: [None]
```

### Run [zipper-2](https://docs.yomo.run/zipper)

```bash
yomo serve -c zipper_2.yaml
cd zipper-2

ℹ️   Running YoMo-Zipper...
2021/11/11 16:09:54 [yomo:zipper] Listening SIGUSR1, SIGUSR2, SIGTERM/SIGINT..
2021/11/11 16:09:54 [core:server] ✅ [zipper-2] Listening on: [::]:9002, QUIC: [v1 draft-29], AUTH: [None]
```

### Run [stream-function](https://docs.yomo.run/stream-fn)

```bash
go run ./sfn/sfn_echo.go

2021/11/11 16:11:05 [core:client] use credential: [None]
2021/11/11 16:11:05 [core:client] ❤️  [echo-sfn]([::]:56245) is connected to YoMo-Zipper localhost:9002
```

### Run [yomo-source](https://docs.yomo.run/source)

```bash
go run ./source/source.go

2021/11/11 16:12:01 [core:client] use credential: [None]
2021/11/11 16:12:01 [core:client] ❤️  [yomo-source] is connected to YoMo-Zipper localhost:9001
2021/11/11 16:12:01 [source] ✅ Emit 1385416436 to YoMo-Zipper
2021/11/11 16:12:01 [source] ✅ Emit 837377611 to YoMo-Zipper
```

### Results

#### stream-function

The terminal of `stream-function` will print the real-time sound value.

```bash
2021/11/11 16:12:01 >> [sfn] got tag=0x33, data=1385416436
2021/11/11 16:12:01 >> [sfn] got tag=0x33, data=837377611
```
