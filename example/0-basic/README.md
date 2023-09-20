# Basic example

This example represents how YoMo works with the mock data of the sound sensor.

## Code structure

- `source`: Mocking data of a Sound Sensor. [docs.yomo.run/source](https://yomo.run/docs/api/source)
- `sfn`: Detecting noise pollution in real-time. [docs.yomo.run/stream-function](https://yomo.run/docs/api/sfn)
- `zipper`: Orchestrate a workflow that receives the data from `source`, stream computing in `stream-fn` [docs.yomo.run/zipper](https://yomo.run/docs/cli/zipper)

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

## Option 2: Manual

### Run [zipper](https://yomo.run/docs/cli/zipper)

```bash
yomo serve -c ../config.yaml

time=2022-12-12T18:12:15.735+08:00 level=INFO msg="Using config file" component=server name=Service file_path=../config.yaml
time=2022-12-12T18:12:15.735+08:00 level=INFO msg="Listening SIGUSR1, SIGUSR2, SIGTERM/SIGINT..."
time=2022-12-12T18:12:15.738+08:00 level=INFO msg=Listening component=server name=Service pid=25220 quic="[v2 v1 draft-29]" auth_name=[none]
```

### Run [stream-function](https://yomo.run/docs/api/sfn)

```bash
go run ./sfn/main.go

2021/11/11 16:11:05 [core:client] use credential: [None]
2021/11/11 16:11:05 [core:client] ❤️  [Noise] is connected to YoMo-Zipper localhost:9000
```

### Run [yomo-source](https://yomo.run/docs/api/source)

```bash
go run ./source/main.go

time=2022-12-12T17:56:27.156+08:00 level=INFO msg="use credential" component=client credential_name=none
time=2022-12-12T17:56:27.161+08:00 level=INFO msg="[source] ✅ Emit to YoMo-Zipper" data="{62.31009 1670838987160 localhost}"
time=2022-12-12T17:56:28.162+08:00 level=INFO msg="[source] ✅ Emit to YoMo-Zipper" data="{58.455963 1670838988161 localhost}"
time=2022-12-12T17:56:29.163+08:00 level=INFO msg="[source] ✅ Emit to YoMo-Zipper" data="{158.80386 1670838989162 localhost}"
time=2022-12-12T17:56:30.164+08:00 level=INFO msg="[source] ✅ Emit to YoMo-Zipper" data="{190.63675 1670838990164 localhost}"
time=2022-12-12T17:56:31.166+08:00 level=INFO msg="[source] ✅ Emit to YoMo-Zipper" data="{147.77885 1670838991166 localhost}"
time=2022-12-12T17:56:32.168+08:00 level=INFO msg="[source] ✅ Emit to YoMo-Zipper" data="{83.59812 1670838992168 localhost}"
```

### Results

#### stream-function

The terminal of `stream-function` will print the real-time sound value.

```bash
time=2022-12-12T18:02:08.408+08:00 level=INFO msg="use credential" component=client credential_name=none
time=2022-12-12T18:02:13.895+08:00 level=INFO msg=[sfn] got=51 data="{98.02577 1670839333894 localhost}"
time=2022-12-12T18:02:14.900+08:00 level=INFO msg=[sfn] got=51 data="{71.31387 1670839334895 localhost}"
time=2022-12-12T18:02:15.898+08:00 level=INFO msg=[sfn] got=51 data="{157.18372 1670839335896 localhost}"
time=2022-12-12T18:02:16.900+08:00 level=INFO msg=[sfn] got=51 data="{13.951344 1670839336898 localhost}"
time=2022-12-12T18:02:17.902+08:00 level=INFO msg=[sfn] got=51 data="{99.50129 1670839337899 localhost}"
time=2022-12-12T18:02:18.904+08:00 level=INFO msg=[sfn] got=51 data="{124.94903 1670839338901 localhost}"
```
