<p align="center">
  <img width="200px" height="200px" src="https://yomo.run/yomo-logo.png" />
</p>

# YoMo应用案例：多数据源的合并计算

## 目标

当有多个高频产生数据的数据源时，我们的客户需要完成一种计算：当所有数据源的数据都到齐后，才进行一次计算任务，并将计算结果传递给下一个处理环节，否则，就一直等待。通常，我们的业务逻辑代码会侵入对多源异构数据的采集、多线程、并发和计算缓存等问题，致使我们不能专心在对业务逻辑的抽象和描述上，而借助YoMo，一切都变得简单起来，您所需要实现的，只有如下几行代码：

```go
var convert = func(v []byte) (interface{}, error) {
	return y3.ToFloat32(v)
}

var zipper = func(_ context.Context, ia interface{}, ib interface{}) (interface{}, error) {
	result := ia.(float32) + ib.(float32)
	return fmt.Sprintf("⚡️ Sum(%s: %f, %s: %f) => Result: %f", "data A", ia.(float32), "data B", ib.(float32), result), nil
}

// Handler handle two event streams and calculate sum when data arrived
func Handler(rxstream rx.Stream) rx.Stream {
	streamA := rxstream.Subscribe(0x11).OnObserve(convert)
	streamB := rxstream.Subscribe(0x12).OnObserve(convert)

	// Rx Zip operator: http://reactivex.io/documentation/operators/zip.html
	stream := streamA.ZipFromIterable(streamB, zipper).StdOut().Encode(0x13)
	return stream
}

```

## 代码结构

+ `source-data-a`: 模拟数据源A，发送随机 Float32 数字. [yomo.run/source](https://yomo.run/source)
+ `source-data-b`: 模拟数据源B，发送随机 Float32 数字. [yomo.run/source](https://yomo.run/source)
+ `stream-fn`（旧名称：flow）: 将模拟数据源A和模拟数据源B进行合并计算[yomo.run/stream-function](https://yomo.run/flow)
+ `yomo-server`（旧名称：zipper）: 设计一个workflow，接收多个source，并完成合并计算 [yomo.run/yomo-server](https://yomo.run/zipper)

## 实现过程

### 1. 安装CLI

```bash
$ go install github.com/yomorun/cli/yomo@latest
```

### 2. 运行 ``yomo-server`

```bash
$ cd ./example/multi-data-source/zipper

$ yomo serve

ℹ️   Found 1 stream functions in yomo-server config
ℹ️   Stream Function 1: training
ℹ️   Running YoMo Server...
2021/03/01 19:05:55 ✅ Listening on 0.0.0.0:9000

```

### 3. 运行 `stream-fn`

> **注意**: `-n` flag 用于表示 flow 的名称, 它需要跟 yomo-server config (workflow.yaml) 里面 function 名称匹配.

```bash
$ cd ./example/multi-data-source/flow

$ yomo run -n training

ℹ️   YoMo Stream Function file: example/multi-data-source/stream-fn/app.go
⌛  Create YoMo Stream Function instance...
ℹ️   Starting YoMo Stream Function instance with Name: Noise. Host: localhost. Port: 9000.
⌛  YoMo Stream Function building...
✅  Success! YoMo Stream Function build.
ℹ️   YoMo Stream Function is running...
2021/03/01 19:05:55 Connecting to zipper localhost:9000 ...
2021/03/01 19:05:55 ✅ Connected to zipper localhost:9000

```

### 3. 运行 `source-data-a`

```bash
$ cd ./example/multi-data-source/source-data-a

$ go run main.go

2021/03/01 17:35:04 ✅ Connected to yomo-server localhost:9000
2021/03/01 17:35:05 ✅ Emit 123.41881 to yomo-server

```

### 4. 运行 `source-data-b`

```bash
$ cd ./example/multi-data-source/source-data-b

$ go run main.go

2021/03/01 17:35:04 ✅ Connected to yomo-server localhost:9000
2021/03/01 17:35:05 ✅ Emit 36.92933 to yomo-server

```

### 5. 观察 `stream-fn` 窗口会有持续不断的数据

```bash
[StdOut]:  ⚡️ Sum(data A: 89.820206, data B: 1651.740967) => Result: 1741.561157
[StdOut]:  ⚡️ Sum(data A: 17.577374, data B: 619.293457) => Result: 636.870850
[StdOut]:  ⚡️ Sum(data A: 114.736366, data B: 964.614075) => Result: 1079.350464
```

这时候，尝试不断的`Ctrl-C`掉`source-data-a`，过一会再启动它，看看`stream-fn`的窗口会有什么变化

### 6. 恭喜您！问题以前所未有的简单的方式解决啦！🚀

更多[使用案例](https://github.com/yomorun/yomo)
