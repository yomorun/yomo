# Basic example

This example represents how YoMo works with the mock data of the sound sensor.

## Code structure

+ `source`: Mocking data of a Sound Sensor. [docs.yomo.run/source](https://docs.yomo.run/source)
+ `sfn`: Detecting noise pollution in real-time. [docs.yomo.run/stream-function](https://docs.yomo.run/stream-fn)
+ `zipper`: Orchestrate a workflow that receives the data from `source`, stream computing in `stream-fn` [docs.yomo.run/zipper](https://docs.yomo.run/zipper)

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

> You could install [task](https://taskfile.dev/#/installation) and run the following steps in one command `task example:basic`.

### 2. Run [zipper](https://docs.yomo.run/zipper)

```bash
yomo serve -c ./zipper/workflow.yaml

Using config file: ./zipper/workflow.yaml
2021/11/11 16:09:54 [yomo:zipper] [AddWorkflow] 0, Noise
ℹ️   Running YoMo-Zipper...
2021/11/11 16:09:54 [yomo:zipper] Listening SIGTERM/SIGINT...
2021/11/11 16:09:54 [core:server] ✅ (name:Service) Listening on: 127.0.0.1:9000, QUIC: [v1 draft-29]
```

### 3. Run [stream-function](https://docs.yomo.run/stream-fn)

```bash
go run ./sfn/main.go

2021/11/11 16:11:05 [core:client] use credential: [None]
2021/11/11 16:11:05 [core:client] ❤️  [Noise] is connected to YoMo-Zipper localhost:9000
```

### 4. Run [yomo-source](https://docs.yomo.run/source)

```bash
go run ./source/main.go

2021/11/11 16:12:01 [core:client] use credential: [None]
2021/11/11 16:12:01 [core:client] ❤️  [yomo-source] is connected to YoMo-Zipper localhost:9000
2021/11/11 16:12:01 [source] ✅ Emit {192.13399 1636618321242 localhost} to YoMo-Zipper
2021/11/11 16:12:01 [source] ✅ Emit {132.86566 1636618321547 localhost} to YoMo-Zipper
2021/11/11 16:12:01 [source] ✅ Emit {199.17604 1636618321851 localhost} to YoMo-Zipper
```

### Results

#### stream-function

The terminal of `stream-function` will print the real-time sound value.

```bash
2021/11/11 16:12:01 >> [sfn] got tag=0x33, data={ 0x1.80449ap+07  0x17d0e0dbd5a 0x6c 0x6f 0x63 0x61 0x6c 0x68 0x6f 0x73 0x74}
2021/11/11 16:12:01 >> [sfn] got tag=0x33, data={ 0x1.09bb38p+07  0x17d0e0dbe8b 0x6c 0x6f 0x63 0x61 0x6c 0x68 0x6f 0x73 0x74}
2021/11/11 16:12:01 >> [sfn] got tag=0x33, data={ 0x1.8e5a22p+07  0x17d0e0dbfbb 0x6c 0x6f 0x63 0x61 0x6c 0x68 0x6f 0x73 0x74}
```
