# YoMo

> Build your own IoT & Edge Realtime Computing system easily, engaging 5G technology

![Go](https://github.com/yomorun/yomo/workflows/Go/badge.svg)

是一个开源项目，方便构建属于您自己的IoT和边缘计算平台。基于YoMo，可快速完成微服务架构的工业App的开发，您的工业互联网平台将会充分发挥5G带来的低延时、大带宽的高通率优势。

## 🚀 3分钟构建工业微服务 Quick Start

### 1. Create a go project and import yomo

```bash
mkdir yomotest && cd yomotest

go mod init yomotest 

go get -u github.com/yomorun/yomo
```

### 2. 编写插件 Start writing your first plugin echo.go

```rust
package main

// 引入yomo
import (
	"github.com/yomorun/yomo/pkg/yomo"
)

func main() {
	// 运行该Plugin，监听:4241端口，数据会被YoMo Edge发送过来
	// yomo.Run(&EchoPlugin{}, "0.0.0.0:4241")
	// 开发调试时的方法，处于联网状态下时，会自动连接至 yomo.run 的开发服务器，连接成功后，
	// 该Plugin会每2秒收到一条Observed()方法指定的Key的Value
	yomo.RunDev(&EchoPlugin{}, "localhost:4241")
}

// EchoPlugin 是一个YoMo Plugin，会将接受到的数据转换成String形式，并再结尾添加内容，修改
// 后的数据将流向下一个Plugin
type EchoPlugin struct{}

// Handle 方法将会在数据流入时被执行，使用Observed()方法通知YoMo该Plugin要关注的key，参数value
// 即该Plugin要处理的内容
func (p *EchoPlugin) Handle(value interface{}) (interface{}, error) {
	return value.(string) + "✅", nil
}

// Observed 返回一个string类型的值，该值是EchoPlugin插件关注的数据流中的Key，该数据流中Key对应
// 的Value将会以对象的形式被传递进Handle()方法中
func (p EchoPlugin) Observed() string {
	return "name"
}

// Name 用于设置该Plugin的名称，方便Debug等操作
func (p *EchoPlugin) Name() string {
	return "EchoPlugin"
}
```

### 3. 运行 Run plugin

1. Open a new termial, run `go run echo.go`, you will see: 

```bash
% go run a.go
[EchoPlugin:6031]2020/07/06 22:14:20 plugin service start... [localhost:4241]
name:yomo!✅
name:yomo!✅
name:yomo!✅
name:yomo!✅
name:yomo!✅
^Csignal: interrupt
```

## 🌟 YoMo架构和亮点

![yomo-arch](https://yomo.run/yomo-arch.png)

### YoMo关注在：

- 工业互联网领域
	- 在IoT设备接入侧，需要<10ms的低延时实时通讯
	- 在智能设备侧，需要在边缘侧进行大算力的AI执行工作
- YoMo包含两部分：
	- `yomo-edge`: 部署在企业内网，负责接收设备数据，并按照配置，依次执行各个`yomo-plugin`
	- `yomo-plugin`: 可以部署在企业私有云、公有云及`yomo-edge-server`上

### YoMo的优势：

- 全程基于Quic协议传输数据，使用UDP协议替代TCP协议后，大幅提升了传输的稳定性和高通率
- 自研的`yomo-codec`优化了数据解码性能
- 全程基于Stream Computing模型，并简化面向Stream编程的复杂度

## 🦸 成为YoMo开发者 Contributing

Github：[github.com/yomorun/yomo](https://github.com/yomorun/yomo)

社区守则：[Code of Conduct](https://github.com/yomorun/yomo/blob/master/CODE_OF_CONDUCT.md)

代码规范：[Contributing Rules](https://github.com/yomorun/yomo/blob/master/CONTRIBUTING.md)

## 🐛 提交Bug

Report bug: [https://github.com/yomorun/yomo/issues](https://github.com/yomorun/yomo/issues/new?assignees=&labels=bug&template=bug_report.md&title=%5BBUG%5D)

## 🧙 Contact Maintainer Team

[yomo@cel.la](mailto:yomo@cel.la)
