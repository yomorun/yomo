# Cascading Zippers

This example represents how YoMo works with cascading zippers in mesh network.

## Code structure

- `source`: Mocking random data and send it to `zipper-1`. [docs.yomo.run/source](https://yomo.run/docs/api/source)
- `zipper-1`: Receive the streams from `source`, and broadcast it to downstream `zipper-2` in another region. [docs.yomo.run/zipper](https://yomo.run/docs/cli/zipper)
- `zipper-2`: Receive the streams from upstream `zipper-1`. [docs.yomo.run/zipper](https://yomo.run/docs/cli/zipper)
- `sfn`: Receive the streams from `zipper-2` and print it in terminal. [docs.yomo.run/stream-function](https://yomo.run/docs/api/sfn)

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

## Option 1: Auto Run

`task run`

```bash
$ task run

task: [zipper-2] go run zipper_2.go
task: [zipper-1] go run zipper_1.go
task: [sfn-build] go build -o ./bin/sfn sfn/sfn_echo.go
task: [source-build] go build -o ./bin/source source/source.go
task: [source] ./bin/source
task: [sfn] ./bin/sfn
[zipper-1] 2022-02-20 17:24:29.822	[core:client] use credential: [None]
[zipper-1] 2022-02-20 17:24:29.822	Server has started!, pid: 49449
[zipper-1] 2022-02-20 17:24:29.823	[yomo:zipper] Listening SIGUSR1, SIGUSR2, SIGTERM/SIGINT...
[zipper-1] 2022-02-20 17:24:29.824	[core:server] ✅ [Zipper-1] Listening on: [::]:9001, QUIC: [v1 draft-29], AUTH: [None]
[zipper-2] 2022-02-20 17:24:29.912	Server has started!, pid: 49450
[zipper-2] 2022-02-20 17:24:29.912	[yomo:zipper] Listening SIGUSR1, SIGUSR2, SIGTERM/SIGINT...
[zipper-2] 2022-02-20 17:24:29.914	[core:server] ✅ [zipper-2] Listening on: 127.0.0.1:9002, QUIC: [v1 draft-29], AUTH: [None]
[zipper-1] 2022-02-20 17:24:30.032	[core:client] ❤️  [zipper-2]([::]:58661) is connected to YoMo-Zipper localhost:9002
[zipper-2] 2022-02-20 17:24:30.032	[core:server] ❤️  <Upstream Zipper> [::zipper-2](127.0.0.1:58661) is connected!
[source] 2022-02-20 17:24:30.229	[core:client] use credential: [None]
[source] 2022-02-20 17:24:30.234	[core:client] ❤️  [yomo-source]([::]:64006) is connected to YoMo-Zipper localhost:9001
[source] 2022-02-20 17:24:30.234	[source] ✅ Emit 2437998737 to YoMo-Zipper
[zipper-1] 2022-02-20 17:24:30.234	[core:server] ❤️  <Source> [::yomo-source](127.0.0.1:64006) is connected!
[sfn] 2022-02-20 17:24:30.549	[core:client] use credential: [None]
[sfn] 2022-02-20 17:24:30.554	[core:client] ❤️  [echo-sfn]([::]:62720) is connected to YoMo-Zipper localhost:9002
[zipper-2] 2022-02-20 17:24:30.555	[core:server] ❤️  <Stream Function> [::echo-sfn](127.0.0.1:62720) is connected!
[source] 2022-02-20 17:24:31.235	[source] ✅ Emit 432890138 to YoMo-Zipper
[sfn] 2022-02-20 17:24:31.238	>> [sfn] got tag=0x33, data=432890138
[source] 2022-02-20 17:24:32.235	[source] ✅ Emit 1245807400 to YoMo-Zipper
[sfn] 2022-02-20 17:24:32.240	>> [sfn] got tag=0x33, data=1245807400
[source] 2022-02-20 17:24:33.236	[source] ✅ Emit 3329942892 to YoMo-Zipper
[sfn] 2022-02-20 17:24:33.239	>> [sfn] got tag=0x33, data=3329942892
[source] 2022-02-20 17:24:34.236	[source] ✅ Emit 2733970616 to YoMo-Zipper
[sfn] 2022-02-20 17:24:34.239	>> [sfn] got tag=0x33, data=2733970616
[source] 2022-02-20 17:24:35.238	[source] ✅ Emit 3313499294 to YoMo-Zipper
[sfn] 2022-02-20 17:24:35.243	>> [sfn] got tag=0x33, data=3313499294

```

## Option 2: Manual

### Run [zipper-1](https://yomo.run/docs/cli/zipper)

```bash
yomo serve -c zipper_1_wf.yaml
```

### Run [zipper-2](https://yomo.run/docs/cli/zipper)

```bash
yomo serve -c zipper_2_wf.yaml
```

### Run [stream-function](https://yomo.run/docs/api/sfn)

```bash
go run ./sfn/sfn_echo.go

2021/11/11 16:11:05 [core:client] use credential: [None]
2021/11/11 16:11:05 [core:client] ❤️  [echo-sfn]([::]:56245) is connected to YoMo-Zipper localhost:9002
```

### Run [yomo-source](https://yomo.run/docs/api/source)

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
