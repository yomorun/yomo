<p align="center">
  <img width="200px" height="200px" src="https://yomo.run/yomo-logo.png" />
</p>

# Use Case：Combined calculation of multiple data sources

## Our customer's asked:

Our client needs to complete a calculation when there are multiple data sources generating data at high frequencies: a calculation task is performed only when all the data from all the data sources have arrived, then send computed result to the next processing session,  otherwise, keeps waiting data. 

Usually, our business logic code intrudes on the collection of heterogeneous data from multiple sources, multi-threading, concurrency and computation caching, which prevents us from concentrating on abstracting and describing the abstraction：

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

## Code structure

+ `source-data-a`: Analog data source A, sending random Float32 numbers. [yomo.run/source](https://yomo.run/source)
+ `source-data-b`: Analog data source B, sending random Float32 numbers. [yomo.run/source](https://yomo.run/source)
+ `flow`: Combine simulated data source A and simulated data source B for calculation[yomo.run/flow](https://yomo.run/flow)
+ `zipper`: Setup a workflow that receives multiple sources and completes the merge calculation [yomo.run/zipper](https://yomo.run/zipper)

## Implementation

### 1. Install CLI

> **Note:** YoMo requires Go 1.15 and above, run `go version` to get the version of Go in your environment, please follow [this link](https://golang.org/doc/install) to install or upgrade if it doesn't fit the requirement.

```bash
# Ensure use $GOPATH, golang requires main and plugin highly coupled
○ echo $GOPATH

```

if `$GOPATH` is not set, check [Set $GOPATH and $GOBIN](#optional-set-gopath-and-gobin) first.

```bash
$ GO111MODULE=off go get github.com/yomorun/yomo

$ cd $GOPATH/src/github.com/yomorun/yomo

$ make install
```

![YoMo Tutorial 1](https://yomo.run/tutorial-1.png)

### 2. Start `flow` for streaming calculating

```bash
$ cd $GOPATH/src/github.com/yomorun/yomo/example/trainingmodel/flow

$ yomo run

2021/03/01 19:01:48 Building the Serverless Function File...
2021/03/01 19:01:49 ✅ Listening on 0.0.0.0:4242

```

### 3. Start `zipper` to orgnize stream processing workflow

```bash
$ cd $GOPATH/src/github.com/yomorun/yomo/example/trainingmodel/zipper

$ yomo wf run

2021/03/01 19:05:55 Found 1 flows in zipper config
2021/03/01 19:05:55 Flow 1: training on localhost:4242
2021/03/01 19:05:55 Found 0 sinks in zipper config
2021/03/01 19:05:55 Running YoMo workflow...
2021/03/01 19:05:55 ✅ Listening on 0.0.0.0:9999

```

### 3. Run `source-data-a`

```bash
$ cd $GOPATH/src/github.com/yomorun/yomo/example/trainingmodel/source-data-a

$ go run main.go

2021/03/01 17:35:04 ✅ Connected to yomo-zipper localhost:9999
2021/03/01 17:35:05 ✅ Emit 123.41881 to yomo-zipper

```

### 4. Run `source-data-b`

```bash
$ cd $GOPATH/src/github.com/yomorun/yomo/example/trainingmodel/source-data-b

$ go run main.go

2021/03/01 17:35:04 ✅ Connected to yomo-zipper localhost:9999
2021/03/01 17:35:05 ✅ Emit 123.41881 to yomo-zipper

```

### 5. `flow` will have a constant flow of output

```bash
[StdOut]:  ⚡️ Sum(data A: 89.820206, data B: 1651.740967) => Result: 1741.561157
[StdOut]:  ⚡️ Sum(data A: 17.577374, data B: 619.293457) => Result: 636.870850
[StdOut]:  ⚡️ Sum(data A: 114.736366, data B: 964.614075) => Result: 1079.350464
```

At this point, try to keep `Ctrl-C` dropping `source-data-a`, start it again after a while and see what happens to the `flow` output

### 6. Congratulations! 

The problem has been solved in a simpler way than ever before! 

Find [More YoMo Use Cases](https://github.com/yomorun/yomo)
