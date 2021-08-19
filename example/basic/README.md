# Basic example

This example represents how YoMo works with the mock data of sound sensor.

## Code structure

- `source`: Mocking data of a Noise Decibel Detection Sensor. [yomo.run/source](https://docs.yomo.run/source)
- `stream-fn` (formerly flow): Detecting noise pollution in real-time and print the warning message when it reaches the threshold. [yomo.run/stream-function](https://docs.yomo.run/stream-fn)
- `stream-fn-db` (formerly sink): Demonstrating persistent storage for IoT data.
- `zipper`: Orchestrate a workflow that receives the data from `source`, stream computing in `stream-fn` [yomo.run/zipper](https://docs.yomo.run/zipper)

## How to run the example

### 1. Install YoMo CLI

Please visit [YoMo Getting Started](https://github.com/yomorun/yomo#1-install-cli) for details.

### 2. Run [zipper](https://docs.yomo.run/zipper)

```bash
yomo serve -c ./zipper/workflow.yaml

ℹ️   Found 1 stream functions in zipper config
ℹ️   Stream Function 1: Noise
ℹ️   Running YoMo Zipper...
```

### 3. Run [stream-function](https://docs.yomo.run/stream-fn)

```bash
yomo run ./stream-fn/app.go -n Noise

ℹ️  YoMo Stream Function file: example/basic/stream-fn/app.go
⌛  Create YoMo Stream Function instance...
ℹ️  Starting YoMo Stream Function instance with Name: Noise. Host: localhost. Port: 9000.
⌛  YoMo Stream Function building...
✅  Success! YoMo Stream Function build.
ℹ️  YoMo Stream Function is running...
2021/05/20 14:10:17 ✅ Connected to zipper localhost:9000
2021/05/20 14:10:17 Running the Stream Function.
```

### (Optional) Run `stream-function` via `Go CLI`

Besides run `stream-function` via `YoMo CLI`, you can also run `stream-function` via `Go CLI` which requires the following additional code snippets in `app.go`:

```go
func main() {
	cli, err := yomo.NewSource(yomo.WithName("Noise")).Connect("localhost", 9000)
	if err != nil {
		log.Print("❌ Connect to zipper failure: ", err)
		return
	}

	defer cli.Close()
	cli.Pipe(Handler)
}
```

You can find the exmaple in `stream-fn-via-go-cli` folder.

```shell
go run ./stream-fn-via-go-cli/app.go

2021/05/21 20:54:52 Connecting to zipper localhost:9000 ...
2021/05/21 20:54:52 ✅ Connected to zipper localhost:9000
```

### 4. Run [stream-fn-db](https://docs.yomo.run/stream-fn)

```bash
go run ./stream-fn-db/app.go -n MockDB

2021/05/20 14:10:29 ✅ Connected to zipper localhost:9000
2021/05/20 14:10:29 Running the Serverless Function.
```

### 5. Run [yomo-source](https://docs.yomo.run/source)

```bash
go run ./source/main.go

2021/05/20 14:11:00 Connecting to zipper localhost:9000 ...
2021/05/20 14:11:00 ✅ Connected to zipper localhost:9000
2021/05/20 14:11:00 ✅ Emit {99.11785 1621491060031 localhost} to zipper
2021/05/20 14:11:00 ✅ Emit {145.5075 1621491060131 localhost} to zipper
2021/05/20 14:11:00 ✅ Emit {118.27067 1621491060233 localhost} to zipper
2021/05/20 14:11:00 ✅ Emit {56.369446 1621491060335 localhost} to zipper
```

### Results

#### stream-function

The terminal of `stream-function` will print the real-time noise decibel value, and show the warning when the value reaches the threshold.

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

#### stream-fn-db

The terminal of `stream-fn-db` will print the message for saving the data in DB.

```bash
save `18.71246` to FaunaDB
save `1.0713108` to FaunaDB
save `16.458117` to FaunaDB
save `12.397432` to FaunaDB
save `15.227814` to FaunaDB
save `14.787642` to FaunaDB
save `17.85902` to FaunaDB
```
