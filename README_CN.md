<p align="center">
  <img width="200px" height="200px" src="https://blog.yomo.run/static/images/logo.png" />
</p>

# YoMo ![Go](https://github.com/yomorun/yomo/workflows/Go/badge.svg)

YoMo 是为边缘计算打造的低时延流式 Serverless 开发框架，基于 [QUIC Transport](https://quicwg.org/) 协议通讯，以 [Functional Reactive Programming](https://en.wikipedia.org/wiki/Functional_reactive_programming) 为编程范式，简化构建可靠、安全的低时延计算应用的复杂度，挖掘5G潜力，释放实时计算价值。

官网：[https://yomo.run](https://docs.yomo.run/?utm_source=github&utm_campaign=ossc) （感谢 <a href="https://vercel.com/?utm_source=cella&utm_campaign=oss" target="_blank">Vercel</a> 支持）

For english, check: [Github](https://github.com/yomorun/yomo)

## 🚀 3分钟教程

### 先决条件

确保已安装 Go 编译运行环境，参考 [安装 Go](https://golang.org/doc/install)

### 1. 安装 CLI

可以通过以下的命令全局安装最新发布的 YoMo CLI：

#### Binary (推荐)

```bash
$ curl -fsSL "https://bina.egoist.sh/yomorun/cli?name=yomo" | sh
```

#### 或者也通过以下方式安装 YoMo CLI

```bash
$ go install github.com/yomorun/cli/yomo@latest
```

或者也可以将 CLI 安装在不同的目录：

```sh
$ env GOBIN=/bin go install github.com/yomorun/cli/yomo@latest
```

#### 验证 CLI 是否成功安装

```bash
$ yomo -V

YoMo CLI version: v0.1.3
```

### 2. 创建第一个yomo应用

```bash
$ yomo init yomo-app-demo

⌛  Initializing the Stream Function...
✅  Congratulations! You have initialized the stream function successfully.
ℹ️   You can enjoy the YoMo Stream Function via the command: 
ℹ️   	DEV: 	yomo dev -n Noise yomo-app-demo/app.go
ℹ️   	PROD: 	First run source application, eg: go run example/source/main.go
		Second: yomo run -n yomo-app-demo yomo-app-demo/app.go

$ cd yomo-app-demo
```

YoMo CLI 会自动创建带有以下内容的 `app.go` 文件：

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/yomorun/yomo/rx"
)

// NoiseData represents the structure of data
type NoiseData struct {
	Noise float32 `json:"noise"` // Noise value
	Time  int64   `json:"time"`  // Timestamp (ms)
	From  string  `json:"from"`  // Source IP
}

var echo = func(_ context.Context, i interface{}) (interface{}, error) {
	value := i.(*NoiseData)
	value.Noise = value.Noise / 10
	rightNow := time.Now().UnixNano() / int64(time.Millisecond)
	fmt.Println(fmt.Sprintf("[%s] %d > value: %f ⚡️=%dms", value.From, value.Time, value.Noise, rightNow-value.Time))
	return value.Noise, nil
}

// Handler will handle data in Rx way
func Handler(rxstream rx.Stream) rx.Stream {
	stream := rxstream.
		Unmarshal(json.Unmarshal, func() interface{} { return &NoiseData{} }).
		Debounce(50).
		Map(echo).
		StdOut()

	return stream
}

func DataID() []byte {
	return []byte{0x33}
}

```

### 3. 调试和运行

1. 为了方便调试，我们创建了一个云端的数据模拟器，它可以产生源源不断的数据，我们只需要运行`yomo dev`就可以看到:

```sh
$ yomo dev

ℹ️   YoMo Stream Function file: app.go
⌛  Create YoMo Stream Function instance...
⌛  YoMo Stream Function building...
✅  Success! YoMo Stream Function build.
ℹ️   YoMo Stream Function is running...
2021/11/16 10:02:43 [core:client]  has connected to yomo-app-demo (dev.yomo.run:9140)
[localhost] 1637028164050 > value: 6.575044 ⚡️=9ms
[StdOut]:  6.5750437
[localhost] 1637028164151 > value: 10.076103 ⚡️=5ms
[StdOut]:  10.076103
[localhost] 1637028164251 > value: 15.560066 ⚡️=8ms
[StdOut]:  15.560066
[localhost] 1637028164352 > value: 15.330824 ⚡️=2ms
[StdOut]:  15.330824
[localhost] 1637028164453 > value: 10.859857 ⚡️=7ms
[StdOut]:  10.859857

```

## 🌶 与更多的优秀开源项目天然集成

### Sources

+ [将 EMQX Broker 连接至 YoMo](https://github.com/yomorun/yomo-source-emqx-starter)
+ [将使用 MQTT 的数据源连接至 YoMo](https://github.com/yomorun/yomo-source-mqtt-broker-starter)

### Stream Functions

+ [基于 WasmEdge 使用 WebAssembly 编写 YoMo-Stream-Function](https://github.com/yomorun/yomo-wasmedge-tensorflow)

### Output Connectors

+ [将 YoMo-Stream-Function 处理完的内容存储至 FaunaDB](https://github.com/yomorun/yomo-sink-faunadb-example)
+ 连接 InfluxDB 落地数据存储
+ [将 YoMo-Stream-Function 处理完的内容存储至 TDengine](https://github.com/yomorun/yomo-sink-tdengine-example)

## 🗺 YoMo系统架构

**Edge-Native**: YoMo 追求随地部署、随时迁移、随时扩容 

![yomo-flow-arch](https://docs.yomo.run/yomo-flow-arch.jpg)

## 📚 Documentation

+ `YoMo-Source`: [docs.yomo.run/source](https://docs.yomo.run/source)
+ `YoMo-Stream-Function`: [docs.yomo.run/stream-function](https://docs.yomo.run/stream-function)
+ `YoMo-Zipper`: [docs.yomo.run/zipper](https://docs.yomo.run/zipper)
+ `Stream Processing in Rx way`: [Rx](https://docs.yomo.run/rx)
+ `Faster than real-time codec`: [Y3](https://github.com/yomorun/y3-codec)

[YoMo](https://yomo.run) ❤️ [Vercel](https://vercel.com/?utm_source=yomorun&utm_campaign=oss), Our documentation website is [https://docs.yomo.run/](https://docs.yomo.run/).

[![Vercel Logo](https://docs.yomo.run/vercel.svg)](https://vercel.com/?utm_source=yomorun&utm_campaign=oss)

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

# QUIC学习资料

![Awesome QUIC Logo](https://gitee.com/fanweixiao/awesome-quic/raw/main/awesome-quic-logo.png)

**QUIC** 的全称是 Quick UDP Internet Connections protocol, 由 Google 设计提出，目前由 IETF 工作组推动进展。其设计的目标是替代 TCP 成为 HTTP/3 的数据传输层协议。熹乐科技在物联网（IoT）和边缘计算（Edge Computing）场景也一直在打造底层基于 QUIC 通讯协议的边缘计算微服务框架 [YoMo](https://yomo.run)，长时间关注 QUIC 协议的发展，遂整理该文集并配以适当的中文翻译，方便更多关注 QUIC 协议的人学习。

# QUIC Weekly - 每周一草

在线社区：🍖[discord/quic](https://discord.gg/CTH3wv9)  
维护者：🦖[YoMo](http://github.com/yomorun/yomo)

## QUIC Weekly - 20210106期

* 微软的QUIC协议实现[MSQUIC v1.0正式发布](https://github.com/microsoft/msquic)
* Web的未来传输通道：[WebTransport Explainer](https://github.com/w3c/webtransport/blob/master/explainer.md)
* [WebTransport](https://w3c.github.io/webtransport/) 的SPEC更新，支持可插拔的协议设计, 开始支持QUIC-TRANSPORT。就像WebSocket一样，但是支持了多通道、 无序传输等特性。
* 史上第一个DNS over QUIC resolver [launched by AdGuard](https://itsecuritywire.com/quick-bytes/worlds-first-dns-over-quic-resolver-launched-by-adguard/)
* [DNS transport: The race is on!](https://centr.org/news/blog/ietf109-dns-transport.html)
* IEEE：[通过基于QUIC的代理功能实现高效的卫星-地面混合传输服务](https://ieeexplore.ieee.org/document/9297334/keywords#keywords)
* [DPIFuzz: 一种用于检测QUIC的DPI模糊策略的差分模糊框架](https://dl.acm.org/doi/pdf/10.1145/3427228.3427662)
* [插件化 QUIC](https://cdn.uclouvain.be/groups/cms-editors-ingi/articles/Pluginzing%20QUIC.pdf)
* [优化协议栈的性能透视: TCP+TLS+HTTP/2 vs. QUIC](https://irtf.org/anrw/2019/anrw2019-final25-acmpaginated.pdf)
* 2018: [WebTransport + WebCodecs at W3C Games Workshop](https://www.w3.org/2018/12/games-workshop/slides/21-webtransport-webcodecs.pdf)
* [qlog 0.4.0 released](crates.io/crates/qlog), 包括对记录原始字节时的流式序列化的修复，以及对DATAGRAM帧记录的改进。

## QUIC Weekly - 20201209期

* Wireshark v3.4.1 发布，[增加了很多与 QUIC 相关的更新](https://www.wireshark.org/docs/relnotes/wireshark-3.4.1.html)
* 📢 [draft-ietf-quic-manageability](https://quicwg.org/ops-drafts/draft-ietf-quic-manageability.html) 讨论了 QUIC 传输协议的可管理性，重点讨论影响 QUIC 流量的网络操作的注意事项，比如，要实现 QUIC 的负载均衡，建议参考该文
* 📢 [Applicability of the QUIC Transport Protocol](https://quicwg.org/ops-drafts/draft-ietf-quic-applicability.html) 讨论了QUIC传输协议的适用性，重点讨论了影响通过QUIC开发和部署应用协议的注意事项，比如，实现0-RTT的过程中要注意的安全问题
* [w3c WebTransport](https://w3c.github.io/webtransport/) 在WebIDL中定义了一组ECMAScript API，允许在浏览器和服务器之间发送和接收数据，在底层实现可插拔协议，在上面实现通用API。本规范使用可插拔的协议，QUIC-TRANSPORT 就是这样一个协议，向服务器发送数据和从服务器接收数据。它可以像WebSockets一样使用，但支持多流、单向流、无序传输、可靠以及不可靠传输。
* 📽 Google 的 David Schinaz 的视频 [QUIC 101](https://www.youtube.com/watch?v=dQ5AND4DPyU)
* Netty [发布了支持 QUIC 的 0.0.1.Final](https://netty.io/news/2020/12/09/quic-0-0-1-Final.html) 该 Codec 实现了 IETF QUIC draft-32 版本，基于 qiuche 项目构建
* Cloudflare 的博客 [为 QUIC 加速 UDP 包传输](https://blog.cloudflare.com/accelerating-udp-packet-transmission-for-quic/)
* [PDF: 软件模拟器 QUIC 协议的性能分析](https://www.researchgate.net/publication/343651688_Performance_analysis_of_Google%27s_Quick_UDP_Internet_Connection_Protocol_under_Software_Simulator)
* 📢 [draft-schinazi-masque-h3-datagram-01](https://tools.ietf.org/html/draft-schinazi-masque-h3-datagram-01) QUIC DATAGRAM 扩展为在 QUIC 上运行的应用协议提供了一种发送不可靠数据的机制，同时利用了QUIC的安全和拥塞控制特性。本文档定义了当在 QUIC 上运行的应用协议是 HTTP/3 时，如何通过在 frame payload 的开头添加一个标识符来使用 QUIC DATAGRAM frame。这允许HTTP消息使用不可靠的DATAGRAM帧来传递相关信息，确保这些帧与HTTP消息正确关联。

## QUIC Weekly - 20201202期

* 📽 Robin Marx 的 [QUIC和HTTP/3的队头阻塞：细节](https://calendar.perfplanet.com/2020/head-of-line-blocking-in-quic-and-http-3-the-details/) [中文版Chinese Version](https://github.com/rmarx/holblocking-blogpost/blob/master/README_CN.md)
* 📽 Hussein Nasser 的 [QUIC之路 - HTTP/1.1、HTTP/2、HTTP Pipelining、CRIME、HTTP/2队头阻塞、HPACK都错在了哪](https://www.youtube.com/watch?v=jp8lvtZa1a8)
* [Netty的实验版开始支持QUIC](https://github.com/netty/netty-incubator-codec-quic) makes use of [quiche](https://github.com/cloudflare/quiche)
* [GnuTLS 3.7.0 开始支持 QUIC 支持](https://blogs.gnome.org/dueno/whats-new-in-gnutls-3-7-0/)

## QUIC Weekly - 20201125期

* Wikipedia 上更新了关于 HTTP/3 的章节：[HTTP/3 - Wikipedia](https://en.wikipedia.org/wiki/HTTP/3)
* [IETF-QUIC 的标准依赖树](https://datatracker.ietf.org/wg/quic/deps/svg/)
* Daniel Stenberg 的新 Keynote [HTTP/3 是下一代 HTTP](https://www2.slideshare.net/bagder/http3-is-next-generation-http?qid=5d7f42ff-797b-4e2f-b4b6-ba223a6afb5a&v=&b=&from_search=1)
* QUIC 在 5G 网络中的实验：[QUIC Throughput and Fairness over Dual Connectivity](https://www.ida.liu.se/~nikca89/papers/mascots20a.slides.pdf)
* [Google's cloud gaming platform Stadia is using QUIC](https://www.reddit.com/r/Stadia/comments/dxam9f/protocol_used_to_stream_games_on_stadia_qos/)
* [跟坚哥学QUIC系列：4 - 连接迁移（Connection Migration）](https://zhuanlan.zhihu.com/p/311221111)
* [跟坚哥学QUIC系列：3 - 加密和传输握手](https://zhuanlan.zhihu.com/p/301505712)
* [跟坚哥学QUIC系列：2 - 地址验证（Address Validation）](https://zhuanlan.zhihu.com/p/290694322)
* [跟坚哥学QUIC系列：1 - 版本协商（Version Negotiation）](https://zhuanlan.zhihu.com/p/286328927)
* 📈 [Builtwith 的 QUIC 应用状况监测](https://trends.builtwith.com/Server/QUIC)

## QUIC Weekly - 20201118期

* 📽 Throwback to [乘坐时光机回到2016年7月QUIC工作组的成立会议](https://www.youtube.com/watch?v=aGvFuvmEufs)，这次会议是基于 Google 当时的实践经验，讨论 QUIC 是否应该成为 IETF 的标准
* 📽 [Robin Marx 讲述 QUIC 和 HTTP/3 的基本功能，开放了他研究的问题及他再 qlog 和 qvis 这两个调试工具上的进展](https://www.youtube.com/watch?v=SuSpghHP0uI&feature=youtu.be)。
* [lsquic 发布了 v2.24.4](https://github.com/litespeedtech/lsquic), 修复了拥塞控制和 CID 生命周期的相关问题。
* [iOS 14 和 macOS Big Sur 包含了 HTTP/3 实验版本的支持](https://developer.apple.com/videos/play/wwdc2020/10111/?time=701) ，并讲述了如何开启 QUIC 的使用，比如在 macOS Big Sur 上，执行: `defaults write -g CFNetworkHTTP3Override -int 3`就可以了。
* Fastly 的官方博客 [《QUIC 成熟时》](https://www.fastly.com/blog/maturing-of-quic)
* 2020-11-16 发布的 [IETF-109 Slide: Tunneling Internet protocols inside QUIC](https://datatracker.ietf.org/meeting/109/materials/slides-109-intarea-tunneling-internet-protocols-inside-quic-00) Rev.00 版本的发布，意味着 QUIC 在整个现有网络生态兼容性的标准迈出的重要一步，这也是为 RFC 标准发布后整体推进而准备。

## QUIC Weekly - 20201111期

* 📢 关于多路复用技术的WG值得关注 **MASQUE Working Group** [Multiplexed Application Substrate over QUIC Encryption (masque)](https://datatracker.ietf.org/wg/masque/about/)

## QUIC Weekly - 20201104期

* 📢 **load-balancers** [Merged了使用POSIX timestamp的PR，这才对嘛](https://github.com/quicwg/load-balancers/pull/56/files)
* 📢 **load-balancers** [draft-ietf-quic-load-balancers-05出来了，相比draft-04的更新参考这里](https://www.ietf.org/rfcdiff?url1=draft-ietf-quic-load-balancers-04&url2=draft-ietf-quic-load-balancers-05)
* **应用** [水果公司的多通道Multipath transport使用场景](https://github.com/quicwg/wg-materials/blob/master/interim-20-10/Multipath%20transports%20at%20Apple.pdf)
* **最佳实践** [IETF QUIC相比HTTP over TLS 1.3 over TCP有显著提升，YouTube缓冲时间降低9%](https://blog.chromium.org/2020/10/chrome-is-deploying-http3-and-ietf-quic.html)
* **最佳实践** [Facebook在视频领域应用QUIC后请求错误率降低8%，卡顿率降低20%](https://engineering.fb.com/2020/10/21/networking-traffic/how-facebook-is-bringing-quic-to-billions/)
* **最佳实践** [Fastly: QUIC and HTTP/3 2020 最新状态](https://zhuanlan.zhihu.com/p/270650394)
* **最佳实践** [Cloudflare: 通往 QUIC 之路（The Road to QUIC）](https://zhuanlan.zhihu.com/p/268171460)
* **知乎** 深入浅出讲解QUIC协议，包含了最近一年的更新 [QUIC 协议简介](https://zhuanlan.zhihu.com/p/276147925)
* **知乎** QUIC的革新带来了后端处理服务的革新机会：[如何设计一款比JSON性能好10倍的编解码器？](https://zhuanlan.zhihu.com/p/274321939)
* **开源** [QUIC 开源实现列表（持续更新）](https://zhuanlan.zhihu.com/p/270628018)
* **开源** [lsquic 2.24.1 发布，@sumams为其增加了新功能，也包含了一些bug修复 🔧.](https://github.com/litespeedtech/lsquic)
* **工具** [Wireshark 3.4.0发布，支持IETF QUIC](https://www.wireshark.org/docs/relnotes/wireshark-3.4.0.html）

## QUIC Weekly - 20201028期

* 📢 [DNS-over-QUIC](https://tools.ietf.org/html/draft-ietf-dprive-dnsoquic-01)：
  * 对科学那啥可是个好东西，太敏感，咱也不敢多说...
* **Paper** [基于QUIC的MQTT协议的实现和分析](https://www.researchgate.net/publication/329835020_Implementation_and_analysis_of_QUIC_for_MQTT)
  * 在端到端的通讯中，确保可靠和安全通信的基础是Transport和Security协议。对于IoT应用，这些协议必须是轻量级的，毕竟IoT设备通常都是硬件能力受限。不幸的是，目前广为流行的TCP/TLS和UDP/DTLS这两种方式，在建连、时延、连接迁移等方面有很多的不足。这篇论文研究了这些缺陷的根源，展示了如何借助QUIC协议优化IoT场景从而达到更高的网络性能，以IoT领域使用范围较广的MQTT协议为例，团队实现了主要的API和功能，并比较了使用QUIC和TCP构建的MQTT协议在有线网络、无线网络和长距离实验场景（long-distance testbeds）中的差异。
  * 测试的结果标明，基于QUIC协议实现的MQTT协议降低建连开销达56%
  * 在半连接场景下，对CPU和内存的消耗分别降低了83%和50%
  * 因为避免了队头阻塞（HOL Blocking）的问题，数据分发时延降低了55%
  * 数据传输速率的抖动也因为QUIC的连接迁移特性得到明显的改善。
* **Article** [HTTP/3: 你需要知道的下一代互联内网协议](https://portswigger.net/daily-swig/http-3-everything-you-need-to-know-about-the-next-generation-web-protocol)
* **Article** [QUIC和物联网IoT](https://calendar.perfplanet.com/2018/quic-and-http-3-too-big-to-fail/)
  * IoT设备是应用QUIC协议的一个好场景，因为这些设备通常工作在无线（蜂窝）网络下（Cellular network），且需要快速建连、0-RTT和重传。但是，这些设备CPU能力普遍较弱。QUIC的作者其实多次提到QUIC对IoT应用场景有很大的提升，可惜的是，至今还没有一套为这个场景设计的协议栈（其实有啊：基于QUIC协议的Edge Computing框架: [🦖YoMo](https://docs.yomo.run/)）
* **Article** [未来的Internet: HTTP/3 — No More TCP, let’s QUIC fix it（谐音梗我翻不出来了...）](https://thexbhpguy.medium.com/the-new-internet-http-3-no-more-tcp-lets-quic-fix-it-6a4cbb6280c7)

## QUIC Weekly - 20201021期

* 📢 QUIC 协议终于出现在 [IETF last call](https://mailarchive.ietf.org/arch/msg/ietf-announce/py1vC4Iuzq18Je4rwF69029oVOI/) 中。
* 📢 QUIC 草案32文件已出：
  * 运输：https://tools.ietf.org/html/draft-ietf-quic-transport-32
  * 恢复：https://tools.ietf.org/html/draft-ietf-quic-recovery-32
  * TLS：https://tools.ietf.org/html/draft-ietf-quic-tls-32
  * HTTP：https://tools.ietf.org/html/draft-ietf-quic-http-32
  * QPACK：https://tools.ietf.org/html/draft-ietf-quic-qpack-19
* **Adoption** 现在 Facebook 已经使用 #QUIC + ＃HTTP3 来处理其全球所有本机应用流量的75％以上！他们从新协议中看到了令人印象深刻的性能提升，尤其是在他们的视频流使用案例中。 [Facebook 如何将 QUIC 带给数十亿人](https://engineering.fb.com/networking-traffic/how-facebook-is-bringing-quic-to-billions/)
* **Adoption** [Node.js 15首次支持 QUIC 和 HTTP/3](https://www.infoworld.com/article/3586354/nodejs-15-debuts-support-for-http3-transport.html)。

## QUIC Weekly - 20201014期

* **Adoption** [Chrome 正在部署 HTTP/3 和 IETF QUIC](https://blog.chromium.org/2020/10/chrome-is-deploying-http3-and-ietf-quic.html)
  * 当前最新的 Google QUIC 版本（Q050）与 IETF QUIC 有很多相似之处。但是到目前为止，大多数 Chrome 用户在未启用某些命令行选项的情况下没有与 IETF QUIC 服务器通信。
  * Google 搜索延迟减少了2％以上。 YouTube 的重新缓冲时间减少了9％以上，而台式机的客户端吞吐量增加了3％以上，移动设备的客户端吞吐量增加了7％以上。我们很高兴地宣布，Chrome 即将推出对 IETF QUIC（特别是草稿版本 H3-29）的支持。
  * 目前，有25％的 Chrome 稳定用户正在使用 H3-29。我们计划在接下来的几周内增加该数字，并继续监控性能数据。
  * Chrome 将积极支持 IETF QUIC H3-29 和 Google QUIC Q050，让支持 Q050 的服务器有时间更新到 IETF QUIC。
* **Adoption** Cloudflare 向用户发送电子邮件，通知从本月开始 [H3 将自动启用](https://cloudflare-quic.com/)。
* CDN 最近被误解了。跨站点的浏览器缓存并不是那么重要，重要的是在存在点（POP）进行缓存。这种 POP 与你的终端用户的距离如此之近，可带来性能提升，因为TCP的传输距离很差。QUIC 可以通过改用 UDP 来解决此问题。 [HackerNews](https://news.ycombinator.com/item?id=24745794)
* **TechTalk** Lucas Pardue：[QUIC 和 HTTP/3：开放标准和开放源代码](https://www.digitalocean.com/community/tech_talks/quic-http-3-open-standards-and-open-source-code) （2020年10月27日。）
* **OpenSource** [quiche](https://github.com/cloudflare/quiche/commit/75c62c1fe97578173b74f16717a7fe9f2d34d5b0) 已支持 QUIC 和 HTTP/3 不可靠的数据报。在保证数据的传输不是最重要的情况下，它可以降低延迟。
* [在 Haskell 中开发 QUIC 丢失检测和拥塞控制](https://kazu-yamamoto.hatenablog.jp/entry/2020/09/15/121613)。
---

# IETF进展

* [draft-ietf-quic-transport-32](https://datatracker.ietf.org/doc/draft-ietf-quic-transport/) QUIC: A UDP-Based Multiplexed and Secure Transport
* [draft-ietf-quic-tls-32](https://datatracker.ietf.org/doc/draft-ietf-quic-tls/) Using TLS to Secure QUIC
* [draft-ietf-quic-invariants-11](https://datatracker.ietf.org/doc/draft-ietf-quic-invariants/) Version-Independent Properties of QUIC
* [draft-ietf-quic-recovery-32](https://datatracker.ietf.org/doc/draft-ietf-quic-recovery/) QUIC Loss Detection and Congestion Control
* [draft-ietf-quic-version-negotiation-01](https://datatracker.ietf.org/doc/draft-ietf-quic-version-negotiation/) Compatible Version Negotiation for QUIC
