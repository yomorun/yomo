# JSON Codec example

This example represents how to use JSON Codec in YoMo.

1. In [source](https://yomo.run/source), use `json.Marshal(data)` to encode the data via JSON.

```go
// Encode data via JSON.
sendingBuf, _ := json.Marshal(data)

// send data to yomo-server via QUIC stream.
_, err := stream.Write(sendingBuf)
```

2. In [stream-fn](https://yomo.run/flow), use `Unmarshal` operator to decode the data via JSON, and then use `Marshal` operator to encode the data back to the stream.

```go
func Handler(rxstream rx.Stream) rx.Stream {
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

### 2. Run [yomo-server](https://yomo.run/zipper)

```bash
yomo serve -c ./yomo-server/workflow.yaml
```

### 3. Run [stream-function](https://yomo.run/flow)

```bash
yomo run ./stream-fn/app.go -n Noise
```

### 4. Run [yomo-source](https://yomo.run/source)

```bash
go run ./source/main.go
```
