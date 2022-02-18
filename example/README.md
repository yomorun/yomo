# Examples

## Linux Pipeline over cloud

- [1-pipeline](https://github.com/yomorun/yomo/tree/master/example/1-pipeline): read the local streams (f.e. `/dev/urandom`) and use [yomo-source](https://docs.yomo.run/source) to send the streams over cloud.
- [2-iopipe](https://github.com/yomorun/yomo/tree/master/example/2-iopipe): use `io.Copy` to pipe the local streams (f.e. `/dev/urandom`) to [yomo-source](https://docs.yomo.run/source).

## Basic example

- [basic](https://github.com/yomorun/yomo/tree/master/example/basic): process the streams from IoT sound sensor.

## Multiple stream functions

- [multi-stream-fn](https://github.com/yomorun/yomo/tree/master/example/multi-stream-fn): use 3 stream functions to process the streams in different cases.
  - stream-fn-1: calculate the sound value in real-time.
  - stream-fn-2: print the warning message when the sound value reaches a threshold.
  - stream-fn-3: calculate the average value in a sliding window.

## Multiple zippers

- [multi-zipper](https://github.com/yomorun/yomo/tree/master/example/multi-zipper): [source](https://docs.yomo.run/source) sends the streams to [zipper-1](https://docs.yomo.run/zipper), then `zipper-1` will broadcast the streams to the zippers in other regions, f.e. `zipper-2`.

## Multiple instances of the same stream function

- [same-stream-fn](https://github.com/yomorun/yomo/tree/master/example/same-stream-fn): multiple instances of the same stream function, `zipper` will send the data to these instances randomly.
