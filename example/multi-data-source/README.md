<p align="center">
  <img width="200px" height="200px" src="https://docs.yomo.run/yomo-logo.png" />
</p>

# Use Case：Combined calculation of multiple data sources

## Our customer's asked:

Our client needs to perform a calculation in an environment where high frequency data generation occurs from multiple data sources. A calculation is only performed when data from all the sources have arrived. After calculation, the computed result is sent to the next processing session, and the whole process repeats. 

Traditionally, in a scenario where heterogenous data from multiple data sources is collected, developers face several issues related to multi-threading, concurrency, race, locking, cache, among other things. As a result, instead of abstraction and implementation, developers spend time fixing issues. YoMo solves that below:

```go
var convert = func(v []byte) (interface{}, error) {
	return y3.ToFloat32(v)
}

var zipper = func(_ context.Context, ia interface{}, ib interface{}) (interface{}, error) {
	result := ia.(float32) + ib.(float32)
	return fmt.Sprintf("⚡️ Sum(%s: %f, %s: %f) => Result: %f", "data A", ia.(float32), "data B", ib.(float32), result), nil
}

// Handler handles two event streams and calculates sum upon data's arrival
func Handler(rxstream rx.RxStream) rx.RxStream {
	streamA := rxstream.Subscribe(0x11).OnObserve(convert)
	streamB := rxstream.Subscribe(0x12).OnObserve(convert)

	// Rx Zip operator: http://reactivex.io/documentation/operators/zip.html
	stream := streamA.ZipFromIterable(streamB, zipper).StdOut().Encode(0x13)
	return stream
}

```

## Code structure

+ `source-data-a`: Analog data source A, sending random Float32 numbers [docs.yomo.run/source](https://docs.yomo.run/source)
+ `source-data-b`: Analog data source B, sending random Float32 numbers [docs.yomo.run/source](https://docs.yomo.run/source)
+ `flow`: Combine simulated data sources A and B for calculation [docs.yomo.run/flow](https://docs.yomo.run/flow)
+ `zipper`: Setup a workflow that receives multiple sources and completes the merge calculation [docs.yomo.run/zipper](https://docs.yomo.run/zipper)

## Implementation

### 1. Install CLI

> **Note:** YoMo requires Go 1.15 and above, run `go version` to get the version of Go in your environment, please follow [this link](https://golang.org/doc/install) to install or upgrade if it doesn't fit the requirement.

```bash
# Ensure use $GOPATH, golang requires main and plugin highly coupled
○ echo $GOPATH

```

if `$GOPATH` is not set, immediately check [Set $GOPATH and $GOBIN](#optional-set-gopath-and-gobin).

```bash
$ GO111MODULE=off go get github.com/yomorun/yomo

$ cd $GOPATH/src/github.com/yomorun/yomo

$ make install
```

### 2. Start `zipper` to organize stream processing workflow

```bash
$ cd $GOPATH/src/github.com/yomorun/yomo/example/multi-data-source/zipper

$ yomo serve

2021/03/01 19:05:55 Found 1 flows in zipper config
2021/03/01 19:05:55 Flow 1: training
2021/03/01 19:05:55 Found 0 sinks in zipper config
2021/03/01 19:05:55 Running YoMo workflow...
2021/03/01 19:05:55 ✅ Listening on 0.0.0.0:9000

```

### 3. Start `flow` for streaming calculation

> **Note**: `-n` flag represents the name of flow, which should match the specific flow in zipper config (workflow.yaml).

```bash
$ cd $GOPATH/src/github.com/yomorun/yomo/example/multi-data-source/flow

$ yomo run -n training

2021/03/01 19:05:55 Building the Serverless Function File...
2021/03/01 19:05:55 Connecting to zipper localhost:9000 ...
2021/03/01 19:05:55 ✅ Connected to zipper localhost:9000
2021/03/01 19:05:55 Running the Serverless Function.

```

### 4. Run `source-data-a`

```bash
$ cd $GOPATH/src/github.com/yomorun/yomo/example/multi-data-source/source-data-a

$ go run main.go

2021/03/01 17:35:04 ✅ Connected to yomo-zipper localhost:9000
2021/03/01 17:35:05 ✅ Emit 123.41881 to yomo-zipper

```

### 5. Run `source-data-b`

```bash
$ cd $GOPATH/src/github.com/yomorun/yomo/example/multi-data-source/source-data-b

$ go run main.go

2021/03/01 17:35:04 ✅ Connected to yomo-zipper localhost:9000
2021/03/01 17:35:05 ✅ Emit 36.92933 to yomo-zipper

```

### 6. `flow` will have a constant flow of output

```bash
[StdOut]:  ⚡️ Sum(data A: 89.820206, data B: 1651.740967) => Result: 1741.561157
[StdOut]:  ⚡️ Sum(data A: 17.577374, data B: 619.293457) => Result: 636.870850
[StdOut]:  ⚡️ Sum(data A: 114.736366, data B: 964.614075) => Result: 1079.350464
```

At this point, try to keep `Ctrl-C` dropping `source-data-a`, start it again after a while and see what happens to the `flow` output

### 7. Congratulations! 

The problem has been solved in a simpler way than ever before! 

Find [More YoMo Use Cases](https://github.com/yomorun/yomo)
