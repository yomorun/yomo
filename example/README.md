# Examples

All examples can be run by [Task](https://taskfile.dev), following the [Install Task](https://taskfile.dev/#/installation), execute `task -l` in this directory will list all the examples.

```bash
task -l |grep example
* example-basic: 			          YoMo basic usage
* example-cascading-zipper: 		Cascading zippers
* example-iopipe: 			        IO Pipe
* example-multi-sfn:      			Multiple stream functions
* example-pipeline:       			Unix pipeline to cloud
```

can run each example directly by `task example-basic`, `task example-cascading-zipper` and etc.

## Basic example

- [0-basic](https://github.com/yomorun/yomo/tree/master/example/0-basic): process the streams from IoT sound sensor.

## Linux Pipeline over cloud

- [1-pipeline](https://github.com/yomorun/yomo/tree/master/example/1-pipeline): read the local streams (f.e. `/dev/urandom`) and use [yomo-source](https://docs.yomo.run/source) to send the streams over cloud.
- [2-iopipe](https://github.com/yomorun/yomo/tree/master/example/2-iopipe): use `io.Copy()` to pipe the local streams (f.e. `/dev/urandom`) to [yomo-source](https://docs.yomo.run/source).

## Multiple stream functions

- [3-multi-sfn](https://github.com/yomorun/yomo/tree/master/example/3-multi-sfn): use 3 stream functions to process the streams in different cases.
  - stream-fn-1: calculate the sound value in real-time.
  - stream-fn-2: print the warning message when the sound value reaches a threshold.
  - stream-fn-3: calculate the average value in a sliding window.

## Cascading zippers

- [4-cascading-zipper](https://github.com/yomorun/yomo/tree/master/example/4-cascading-zipper): [source](https://docs.yomo.run/source) connect to [zipper-1](https://docs.yomo.run/zipper), then [zipper-1](https://docs.yomo.run/zipper) will broadcast the streams to the zippers in other regions.
