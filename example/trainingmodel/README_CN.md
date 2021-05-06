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
func Handler(rxstream rx.RxStream) rx.RxStream {
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
+ `flow`: 将模拟数据源A和模拟数据源B进行合并计算[yomo.run/flow](https://yomo.run/flow)
+ `zipper`: 设计一个workflow，接收多个source，并完成合并计算 [yomo.run/zipper](https://yomo.run/zipper)

## 实现过程

### 1. 安装CLI

> **注意：** YoMo 的运行环境要求 Go 版本为 1.15 或以上，运行 `go version` 获取当前环境的版本，如果未安装 Go 或者不符合 Go 版本要求时，请安装或者升级 Go 版本。
安装 Go 环境之后，国内用户可参考 <https://goproxy.cn/> 设置 `GOPROXY`，以便下载 YoMo 项目依赖。

```bash
# 确保设置了$GOPATH, Golang的设计里main和plugin是高度耦合的
$ echo $GOPATH

```

如果没有设置`$GOPATH`，参考这里：[如何设置$GOPATH和$GOBIN](#optional-set-gopath-and-gobin)。

```bash
$ GO111MODULE=off go get github.com/yomorun/yomo

$ cd $GOPATH/src/github.com/yomorun/yomo

$ make install
```

![YoMo Tutorial 1](https://yomo.run/tutorial-1.png)

### 2. 运行 `flow`

```bash
$ cd $GOPATH/src/github.com/yomorun/yomo/example/trainingmodel/flow

$ yomo run

2021/03/01 19:01:48 Building the Serverless Function File...
2021/03/01 19:01:49 ✅ Listening on 0.0.0.0:4242

```

### 3. 运行 `zipper`

```bash
$ cd $GOPATH/src/github.com/yomorun/yomo/example/trainingmodel/zipper

$ yomo wf run

2021/03/01 19:05:55 Found 1 flows in zipper config
2021/03/01 19:05:55 Flow 1: training on localhost:4242
2021/03/01 19:05:55 Found 0 sinks in zipper config
2021/03/01 19:05:55 Running YoMo workflow...
2021/03/01 19:05:55 ✅ Listening on 0.0.0.0:9999

```

### 3. 运行 `source-data-a`

```bash
$ cd $GOPATH/src/github.com/yomorun/yomo/example/trainingmodel/source-data-a

$ go run main.go

2021/03/01 17:35:04 ✅ Connected to yomo-zipper localhost:9999
2021/03/01 17:35:05 ✅ Emit 123.41881 to yomo-zipper

```

### 4. 运行 `source-data-b`

```bash
$ cd $GOPATH/src/github.com/yomorun/yomo/example/trainingmodel/source-data-b

$ go run main.go

2021/03/01 17:35:04 ✅ Connected to yomo-zipper localhost:9999
2021/03/01 17:35:05 ✅ Emit 123.41881 to yomo-zipper

```

### 5. 观察 `flow` 窗口会有持续不断的数据

```bash
[StdOut]:  ⚡️ Sum(data A: 89.820206, data B: 1651.740967) => Result: 1741.561157
[StdOut]:  ⚡️ Sum(data A: 17.577374, data B: 619.293457) => Result: 636.870850
[StdOut]:  ⚡️ Sum(data A: 114.736366, data B: 964.614075) => Result: 1079.350464
```

这时候，尝试不断的`Ctrl-C`掉`source-data-a`，过一会再启动它，看看`flow`的窗口会有什么变化

### 6. 恭喜您！问题以前所未有的简单的方式解决啦！🚀

更多[使用案例](https://github.com/yomorun/yomo)
