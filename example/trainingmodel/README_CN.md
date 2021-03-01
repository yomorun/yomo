<p align="center">
  <img width="200px" height="200px" src="https://yomo.run/yomo-logo.png" />
</p>

## 模拟AI模型训练案例
### 简介
#### 当数据A和数据B都到达flow，才进行数据AI训练
### 目录
+ `source-data-a`: 模拟数据A，发送随机 Float32 数字. [yomo.run/source](https://yomo.run/source)
+ `source-data-b`: 模拟数据B，发送随机 Float32 数字. [yomo.run/source](https://yomo.run/source)
+ `flow`: 将模拟数据A和模拟数据B进行合并模拟AI训练[yomo.run/flow](https://yomo.run/flow)
+ `zipper`: 接收多个source [yomo.run/zipper](https://yomo.run/zipper)


## 🚀 1分钟教程

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
[data-a]> value: 123.418808
[data-a]> value: 61.735325
[data-b]> value: 1527.041382
[StdOut]:  ⚡️ Zip [dataA],[dataB] -> Value: 123.418808, 1527.041382
```
### 6. 恭喜您！此项目已经完美运行起来啦！🚀

### Optional: Set $GOPATH and $GOBIN

针对Terminal当前的Session:

```bash
export GOPATH=~/.go
export PATH=$GOPATH/bin:$PATH
```

Shell用户持久保存配置设置: 

如果您是`zsh`用户：

```bash
echo "export GOPATH=~/.go" >> .zshrc
echo "path+=$GOPATH/bin" >> .zshrc
```

如果您是`bash`用户：

```bash
echo 'export GOPATH=~/.go' >> .bashrc
echo 'export PATH="$GOPATH/bin:$PATH"' >> ~/.bashrc
```

## 🌶 与更多的优秀开源项目天然集成

### Sources

+ [将 EMQX Broker 连接至 YoMo](https://github.com/yomorun/yomo-source-emqx-starter)
+ [将使用 MQTT 的数据源连接至 YoMo](https://github.com/yomorun/yomo-source-mqtt-broker-starter)

### Flows

+ [基于 SSVM 使用 WebAssembly 编写 YoMo-Flow](https://github.com/yomorun/yomo-flow-ssvm-example)

### Sinks

+ [将 YoMo-Flow 处理完的内容存储至 FaunaDB](https://github.com/yomorun/yomo-sink-faunadb-example)
+ 连接 InfluxDB 落地数据存储
+ [将 YoMo-Flow 处理完的内容存储至 TDengine](https://github.com/yomorun/yomo-sink-tdengine-example)

## 🗺 YoMo系统架构

**Edge-Native**: YoMo 追求随地部署、随时迁移、随时扩容 

![yomo-flow-arch](https://yomo.run/yomo-flow-arch.jpg)

## 📚 Documentation

+ `YoMo-Source`: [yomo.run/source](https://yomo.run/source)
+ `YoMo-Flow`: [yomo.run/flow](https://yomo.run/flow)
+ `YoMo-Sink`: [yomo.run/sink](https://yomo.run/sink)
+ `YoMo-Zipper`: [yomo.run/zipper](https://yomo.run/zipper)
+ `Stream Processing in Rx way`: [Rx](https://yomo.run/rx)
+ `Faster than real-time codec`: [Y3](https://github.com/yomorun/y3-codec)

[YoMo](https://yomo.run) ❤️ [Vercel](https://vercel.com/?utm_source=yomorun&utm_campaign=oss), Our documentation website is

![Vercel Logo](https://raw.githubusercontent.com/yomorun/yomo-docs/main/public/vercel.svg)

## 🎯 越来越多的数据产生在数据中心之外，YoMo 关注在离数据更近的位置，提供便利的计算框架

- 对时延敏感的场景
- 蜂窝网络下的会出现性能抖动，存在丢包、延时，比如LTE、5G
- 源源不断的高频数据涌向业务处理
- 对于复杂系统，希望使用 Streaming-Serverless 架构简化

## 🌟 YoMo 优势：

- 全程基于 QUIC 协议传输数据，使用UDP协议替代TCP协议后，大幅提升了传输的稳定性和高通率
- 自研的`yomo-codec`优化了数据解码性能
- 全程基于 Rx 实现 Stream Computing 模型，并简化面向流式编程的复杂度
- 通讯协议级别的“本质安全”

## 🦸 成为 YoMo 贡献者

首先感谢您的 contributions，是您这样的人让 YoMo 能变得越来越好！参与 YoMo 项目有很多种方式：

- [提交bug🐛](https://github.com/yomorun/yomo/issues/new?assignees=&labels=bug&template=bug_report.md&title=%5BBUG%5D)，请务必记得描述您所运行的YoMo的版本、操作系统和复现bug的步骤。

- 建议新的功能

- 在贡献代码前，请先阅读[Contributing Guidelines](https://gitee.com/yomorun/yomo/blob/master/CONTRIBUTING.md) 

- 当然我们也有 [Code of Conduct](https://gitee.com/yomorun/yomo/blob/master/CODE_OF_CONDUCT.md)

## 🤹🏻‍♀️ 反馈和建议

任何时候，建议和意见都可以写在 [Discussion](https://github.com/yomorun/yomo/discussions)，每一条反馈都一定会被社区感谢！

## 开源协议

[Apache License 2.0](http://www.apache.org/licenses/LICENSE-2.0.html)
