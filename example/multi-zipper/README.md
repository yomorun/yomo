# Cascading Zippers

This example represents how YoMo works with cascading zippers in mesh network.

## Code structure

+ `source`: Mocking random data and send it to `zipper-1`. [docs.yomo.run/source](https://docs.yomo.run/source)
+ `zipper-1`: Receive the streams from `source`, and broadcast it to downstream `zipper-2` in another region. [docs.yomo.run/zipper](https://docs.yomo.run/zipper)
+ `zipper-2`: Receive the streams from upstream `zipper-1`. [docs.yomo.run/zipper](https://docs.yomo.run/zipper)
+ `sfn`: Receive the streams from `zipper-2` and print it in terminal. [docs.yomo.run/stream-function](https://docs.yomo.run/stream-fn)

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

> You could install [task](https://taskfile.dev/#/installation) and run the following steps in one command `task example:multi-zipper`.

### 2. Run [zipper-1](https://docs.yomo.run/zipper)

```bash
cd zipper-1
go run zipper_1.go

ℹ️   Running YoMo-Zipper...
2021/11/11 16:09:54 [yomo:zipper] Listening SIGUSR1, SIGUSR2, SIGTERM/SIGINT..
2021/11/11 16:09:54 [core:server] ✅ [Zipper-1] Listening on: [::]:9001, QUIC: [v1 draft-29], AUTH: [None]
```

### 3. Run [zipper-2](https://docs.yomo.run/zipper)

```bash
cd zipper-2
go run zipper_2.go

ℹ️   Running YoMo-Zipper...
2021/11/11 16:09:54 [yomo:zipper] Listening SIGUSR1, SIGUSR2, SIGTERM/SIGINT..
2021/11/11 16:09:54 [core:server] ✅ [zipper-2] Listening on: [::]:9002, QUIC: [v1 draft-29], AUTH: [None]
```

### 3. Run [stream-function](https://docs.yomo.run/stream-fn)

```bash
go run ./sfn/sfn_echo.go

2021/11/11 16:11:05 [core:client] use credential: [None]
2021/11/11 16:11:05 [core:client] ❤️  [echo-sfn]([::]:56245) is connected to YoMo-Zipper localhost:9002
```

### 4. Run [yomo-source](https://docs.yomo.run/source)

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
