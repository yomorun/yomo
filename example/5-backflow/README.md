# Backflow example

This example represents how [source](https://docs.yomo.run/source) receives stream functions processed results.

## Code structure

+ `source`: Mocking data of a Sound Sensor. [docs.yomo.run/source](https://docs.yomo.run/source)
+ `sfn-1`: Convert the noise value to `int` type in real-time. [docs.yomo.run/stream-function](https://docs.yomo.run/stream-fn)
+ `sfn-2`: Calculate 10 times the noise value in real-time. [docs.yomo.run/stream-function](https://docs.yomo.run/stream-fn)
+ `zipper`: Orchestrate a workflow that receives the data from `source`, stream computing in `stream-fn` [docs.yomo.run/zipper](https://docs.yomo.run/zipper)

## Prepare

Install YoMo CLI

### Binary (Recommended)

```bash
$ curl -fsSL "https://bina.egoist.sh/yomorun/cli?name=yomo" | sh

  ==> Resolved version latest to v1.1.0
  ==> Downloading asset for darwin amd64
  ==> Installing yomo to /usr/local/bin
  ==> Installation complete
```

### Or build from source

```bash
$ go install github.com/yomorun/cli/yomo@latest
$ yomo version
YoMo CLI Version: v1.1.0
Runtime Version: v1.8.0
```

## Option 1: Auto Run

`task run`

## Option 2: Manual

### Run [zipper](https://docs.yomo.run/zipper)

```bash
yomo serve -c ./workflow.yaml

2022-06-13 15:46:01.477 [yomo:zipper] Listening SIGUSR1, SIGUSR2, SIGTERM/SIGINT...
2022-06-13 15:46:01.479 [core:server] ✅ [backflow][71590] Listening on: 127.0.0.1:9000, MODE: DEVELOPMENT, QUIC: [v1 draft-29], AUTH: [none]
```

### Run [sfn-1](https://docs.yomo.run/stream-fn)

```bash
go run ./sfn-1/main.go

2022-06-13 15:53:17.486 [core:client] use credential: [none]
2022-06-13 15:53:17.496 [core:client] ❤️  [sfn-1][e6KHnVWboNz0x8Ffhvq-e]([::]:56117) is connected to YoMo-Zipper localhost:9000
```
### Run [sfn-2](https://docs.yomo.run/stream-fn)
```bash
go run ./sfn-2/main.go

2022-06-13 15:53:17.486 [core:client] use credential: [none]
2022-06-13 15:53:17.496 [core:client] ❤️  [sfn-2][e6KHnVWboNz0x8Ffhvq-e]([::]:56117) is connected to YoMo-Zipper localhost:9000
```

### Run [yomo-source](https://docs.yomo.run/source)

```bash
go run ./source/main.go

2022-06-13 16:00:10.440 [core:client] use credential: [none]
2022-06-13 16:00:10.447 [core:client] ❤️  [yomo-source][QqkNxX3tQlnw64Pg8JqZR]([::]:64036) is connected to YoMo-Zipper localhost:9000
...
```

### Results

The terminal of `yomo-srouce` will print the real-time receives value.

```bash
2022-06-13 16:06:48.690 [source] ✅ Emit 158.30 to YoMo-Zipper
2022-06-13 16:06:48.691 [source] ♻️  receive backflow: tag=0x34, data=158
2022-06-13 16:06:48.692 [source] ♻️  receive backflow: tag=0x35, data=1580
2022-06-13 16:06:49.691 [source] ✅ Emit 28.81 to YoMo-Zipper
2022-06-13 16:06:49.693 [source] ♻️  receive backflow: tag=0x34, data=28
2022-06-13 16:06:49.694 [source] ♻️  receive backflow: tag=0x35, data=280
2022-06-13 16:06:50.691 [source] ✅ Emit 3.81 to YoMo-Zipper
2022-06-13 16:06:50.694 [source] ♻️  receive backflow: tag=0x34, data=3
2022-06-13 16:06:50.694 [source] ♻️  receive backflow: tag=0x35, data=30
...
```

The terminal of `sfn-1` will print the real-time noise value.

```bash
2022-06-13 16:06:48.691 [sfn-1] got: tag=0x33, data=158.3, return: tag=0x34, data=158
2022-06-13 16:06:49.692 [sfn-1] got: tag=0x33, data=28.81, return: tag=0x34, data=28
2022-06-13 16:06:50.693 [sfn-1] got: tag=0x33, data=3.81, return: tag=0x34, data=3
...
```

The terminal of `sfn-2` will print the real-time noise value.

```bash
2022-06-13 16:06:48.692 [sfn-2] got: tag=0x34, data=158, return: tag=0x35, data=1580
2022-06-13 16:06:49.693 [sfn-2] got: tag=0x34, data=28, return: tag=0x35, data=280
2022-06-13 16:06:50.694 [sfn-2] got: tag=0x34, data=3, return: tag=0x35, data=30
...
```



