# Pipe stream functions example

This example represents how YoMo works with sequential pipe data.

## Code structure

+ `source`: Send data with incremental ID. [yomo.run/source](https://docs.yomo.run/source)
+ `stream-fn-1` (formerly flow): The pipe function receives data as the original order, and after a random period of time it will send the data to downstream sfn. [yomo.run/stream-function](https://docs.yomo.run/stream-function)
+ `stream-fn-2` (formerly flow): Print the received data ID. [yomo.run/stream-function](https://docs.yomo.run/stream-function)
+ `zipper`: Orchestrate a workflow that receives the data from `source`, stream computing in `stream-fn-1` and `stream-fn-2` [yomo.run/zipper](https://docs.yomo.run/zipper)

## How to run the example

### 1. Install YoMo CLI

Please visit [YoMo Getting Started](https://github.com/yomorun/yomo#1-install-cli) for details.

### 2. Build

```bash
cd ..
task pipe-stream-fn-build
```

### 3. Run [YoMo-Zipper](https://docs.yomo.run/zipper)

```bash
yomo serve -c workflow.yaml
```

### 4. Run [stream-fn-1](https://docs.yomo.run/stream-function)

```bash
./stream-fn-1/fn1
```

### 5. Run [stream-fn-2](https://docs.yomo.run/stream-function)

```bash
./stream-fn-2/fn2
```

### 6. Run [yomo-source](https://docs.yomo.run/source)

```bash
./source/source
```

### Results

The terminal of `stream-fn-1` will print the sleep time.

```bash
2022-01-24 14:47:38.716	✅ Sleep: 2112 ms
2022-01-24 14:47:40.829	✅ Sleep: 899 ms
2022-01-24 14:47:41.729	✅ Sleep: 1389 ms
2022-01-24 14:47:43.119	✅ Sleep: 2075 ms
2022-01-24 14:47:45.196	✅ Sleep: 1562 ms
2022-01-24 14:47:46.759	✅ Sleep: 883 ms
```

The terminal of `stream-fn-2` will receive data in order.

```bash
2022-01-24 14:47:40.830	✅ Receive: 1
2022-01-24 14:47:41.730	✅ Receive: 2
2022-01-24 14:47:43.120	✅ Receive: 3
2022-01-24 14:47:45.197	✅ Receive: 4
2022-01-24 14:47:46.760	✅ Receive: 5
2022-01-24 14:47:47.644	✅ Receive: 6
```
