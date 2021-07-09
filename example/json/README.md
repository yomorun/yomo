# JSON Codec example

This example represents how to use JSON Codec in YoMo.

1. In [source](https://docs.yomo.run/source), use `json.Marshal(data)` to encode the data via JSON.

```go
// Encode data via JSON.
sendingBuf, _ := json.Marshal(data)

// send data to zipper via QUIC stream.
_, err := stream.Write(sendingBuf)
```

2. In [flow](https://docs.yomo.run/flow), use `Unmarshal` operator to decode the data via JSON, and then use `Marshal` operator to encode the data back to the stream.

```go
func Handler(rxstream rx.RxStream) rx.RxStream {
	stream := rxstream.
		Unmarshal(json.Unmarshal, func() interface{} { return &NoiseData{} }).
		Map(computePeek).
		Marshal(json.Marshal)

	return stream
}
```

## How to run the example

### 1. Install YoMo CLI

Please visit [YoMo Getting Started](https://github.com/yomorun/yomo#1-install-cli) for details.

### 2. Run [yomo-zipper](https://docs.yomo.run/zipper)

```bash
yomo serve -c ./zipper/workflow.yaml
```

### 3. Run [yomo-flow](https://docs.yomo.run/flow)

```bash
yomo run ./flow/app.go -n Noise
```

### 4. Run [yomo-source](https://docs.yomo.run/source)

```bash
go run ./source/main.go
```
