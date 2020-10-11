# YoMo ![Go](https://github.com/yomorun/yomo/workflows/Go/badge.svg)

YoMo 是一套开源的实时边缘计算网关、开发框架和微服务平台，通讯层基于 [QUIC](https://en.wikipedia.org/wiki/QUIC) 协议 ([2020-09-25更新到Draft-31版本](https://tools.ietf.org/html/draft-ietf-quic-transport-31))，更好的释放了 5G 等下一代低时延网络的价值。为流式处理（Streaming Computing）设计的编解码器`yomo-codec`能大幅提升计算服务的吞吐量；基于插件的开发模式，5分钟即可上线您的物联网实时边缘计算处理系统。YoMo 目前已在工业互联网领域被部署应用。

官网： [https://yomo.run](https://yomo.run/)

## 💘 QUIC快速学习资源 Awesome QUIC

* 不在爱了 TCP 💔:
	* [为什么TCP是个烂协议](https://zhuanlan.zhihu.com/p/20144829)
	* 今天 TCP 烂了怎么办？[如何看待谷歌 Google 打算用 QUIC 协议替代 TCP/UDP？](https://www.zhihu.com/question/29705994)
* 浅尝 QUIC 科普贴 🎱：
	* 知乎腾讯技术官号 [科普：QUIC协议原理分析](https://zhuanlan.zhihu.com/p/32553477)
	* [新一代互联网传输协议QUIC浅析](https://zhuanlan.zhihu.com/p/76202865)
* 真干实践大厂贴 🏌️‍♂️:
	* 腾讯 QUIC 实践 [让互联网更快的协议，QUIC在腾讯的实践及性能优化](https://zhuanlan.zhihu.com/p/32560981)
	* 阿里 QUIC 实践 
		* [阿里XQUIC：标准QUIC实现自研之路](https://mp.weixin.qq.com/s/pBv_DnG05YWl4ZYRHThaTw)
		* [AliQUIC：场景化高性能传输网络实践](https://developer.aliyun.com/article/643770)
	* 七牛 QUIC 实践 [流畅度提高 100%！七牛云 QUIC 推流方案如何实现直播 0 卡顿](https://zhuanlan.zhihu.com/p/33698793)
	* 又拍云 QUIC 实践 [QUIC协议详解之Initial包的处理](https://zhuanlan.zhihu.com/p/162914823)
	* 微博 QUIC 实践 [QUIC在微博中的落地思考](https://www.infoq.cn/article/2018/03/weibo-quic)
	* B站 QUIC 实践 [B站QUIC实践之路](https://mp.weixin.qq.com/s/DrGm-OkSpJbzPWbFmSBT8g)
	* Facebook QUIC 实践 [Building Zero protocol for fast, secure mobile connections](https://engineering.fb.com/networking-traffic/building-zero-protocol-for-fast-secure-mobile-connections/)
	* Cloudflare QUIC 实践 [The Road to QUIC](https://blog.cloudflare.com/the-road-to-quic/)
	* Uber QUIC 实践
		* [Employing QUIC Protocol to Optimize Uber’s App Performance](https://eng.uber.com/employing-quic-protocol/)
		* [Uber Networking: Challenges and Opportunities](https://www.slideshare.net/dhaval2025/uber-mobility-high-performance-networking)
	* Fastly QUIC 实践 [Modernizing the internet with HTTP/3 and QUIC](https://www.fastly.com/blog/modernizing-the-internet-with-http3-and-quic)
* 熬夜充电技术细节贴 🦾:
	* [让互联网更快的“快”---QUIC协议原理分析](https://zhuanlan.zhihu.com/p/32630510)
	* [QUIC 是如何做到 0RTT 的](https://zhuanlan.zhihu.com/p/142794794)
	* [快速理解为什么说UDP有时比TCP更有优势](http://www.52im.net/thread-1277-1-1.html)
	* [一泡尿的时间，快速读懂QUIC协议](http://www.52im.net/thread-2816-1-1.html)
* 墙裂推荐英文贴 🍿:
	* 🍿 QUIC工作组主席 [Lars Eggert博士](https://eggert.org/) 的 [QUIC: a new internet transport](https://video.fsmpi.rwth-aachen.de/17ws-quic/12107) (🎬 58:39) @2017
	* 🍿 谷歌官方 2014 年发布的视频 [QUIC: next generation multiplexed transport over UDP](https://www.youtube.com/watch?v=hQZ-0mXFmk8) (🎬 51:40) @2014
	* F5 首席架构师 Jason Rahm [What is QUIC?](https://www.youtube.com/watch?v=RIFnXaiRs_o) (🎬 08:35) @2018
	* Codevel博客文章 [https://medium.com/codavel-blog/quic-vs-tcp-tls-and-why-quic-is-not-the-next-big-thing-d4ef59143efd](https://medium.com/codavel-blog/quic-vs-tcp-tls-and-why-quic-is-not-the-next-big-thing-d4ef59143efd)
* 估计你们不会看的🧟‍♀️:
	* QUIC: A UDP-Based Multiplexed and Secure Transport [draft-ietf-quic-transport-31](https://datatracker.ietf.org/doc/draft-ietf-quic-transport/)
	* Using TLS to Secure QUIC [draft-ietf-quic-tls-31](https://datatracker.ietf.org/doc/draft-ietf-quic-tls/)
	* Version-Independent Properties of QUIC [draft-ietf-quic-invariants-11](https://datatracker.ietf.org/doc/draft-ietf-quic-invariants/)
	* QUIC Loss Detection and Congestion Control [draft-ietf-quic-recovery-31](https://datatracker.ietf.org/doc/draft-ietf-quic-recovery/)
	* Compatible Version Negotiation for QUIC [draft-ietf-quic-version-negotiation-01](https://datatracker.ietf.org/doc/draft-ietf-quic-version-negotiation/)

## 🚀 3分钟构建工业微服务 Quick Start

### 1. 创建工程，并引入yomo

创建一个叫`yomotest`的目录：

```bash
mkdir yomotest
cd yomotest
```

初始化项目：

```
go mod init yomotest
```

引入yomo

```
go get -u gitee.com/yomorun/yomo
```

### 2. 编写业务逻辑`echo.go`

```go
package main

import (
	"github.com/yomorun/yomo/pkg/yomo"
)

func main() {
  //// 运行echo plugin并监控4241端口，数据将会从YoMo Edge推送过来
  // yomo.Run(&EchoPlugin{}, "0.0.0.0:4241")
	
  // 开发调试时运行该方法，处于联网状态时，程序会自动连接至 yomo.run 的开发服务器，连接成功后，
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

// Mold 描述`Observed`的值的数据结构
func (p EchoPlugin) Mold() interface{} {
	return ""
}
```

### 3. 运行

1. 在终端里执行 `go run echo.go`，您将会看到：

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
恭喜！您的第一个YoMo应用已经完成！

小提示: 如果您使用复合数据结构（Complex Mold）, 请参考：[yomo-echo-plugin](https://gitee.com/yomorun/yomo-echo-plugin)。

## 🌟 YoMo架构和亮点

![yomo-arch](https://yomo.run/yomo-arch.png)

### YoMo关注在：

- 工业互联网领域
  - 在IoT设备接入侧，需要<10ms的低延时实时通讯
  - 在智能设备侧，需要在边缘侧进行大算力的AI执行工作
- YoMo 包含两部分：
  - `yomo-edge`: 部署在企业内网，负责接收设备数据，并按照配置，依次执行各个`yomo-plugin`
  - `yomo-plugin`: 可以部署在企业私有云、公有云及 YoMo Edge Server 上

### YoMo的优势：

- 全程基于 QUIC 协议传输数据，使用UDP协议替代TCP协议后，大幅提升了传输的稳定性和高通率
- 自研的`yomo-codec`优化了数据解码性能
- 全程基于Stream Computing模型，并简化面向Stream编程的复杂度

## 🦸 成为YoMo开发者

First off, thank you for considering making contributions. It's people like you that make YoMo better. There are many ways in which you can participate in the project, for example:
首先感谢您的contributions，是您这样的人让YoMo能变得越来越好！参与YoMo项目有很多种方式：

- [提交bug🐛](https://github.com/yomorun/yomo/issues/new?assignees=&labels=bug&template=bug_report.md&title=%5BBUG%5D)，请务必记得描述您所运行的YoMo的版本、操作系统和复现bug的步骤。

- 建议新的功能

- 在贡献代码前，请先阅读[Contributing Guidelines](https://gitee.com/yomorun/yomo/blob/master/CONTRIBUTING.md) 

- 当然我们也有 [Code of Conduct](https://gitee.com/yomorun/yomo/blob/master/CODE_OF_CONDUCT.md)

##  🧙 联系YoMo组织

Email us at [yomo@cel.la](mailto:yomo@cel.la). Any feedback would be greatly appreciated!

## 开源协议

[Apache License 2.0](http://www.apache.org/licenses/LICENSE-2.0.html)
