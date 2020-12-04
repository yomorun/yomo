# YoMo ![Go](https://github.com/yomorun/yomo/workflows/Go/badge.svg)

YoMo 是为边缘计算打造的低时延流式 Serverless 开发框架，基于 [QUIC Transport](https://quicwg.org/) 协议通讯，以 [Functional Reactive Programming](https://en.wikipedia.org/wiki/Functional_reactive_programming) 为编程范式，简化构建可靠、安全的低时延计算应用的复杂度，挖掘5G潜力，释放实时计算价值。

5G 和 AI 的发展，带来数据的爆发式增长，和对数据的实时计算需求。从 VR/AR游戏和云游戏、超清视频，到智能制造、远程医疗和自动驾驶，低时延应用正在各行各业涌现。YoMo 提供的低时延流式计算框架，致力于简化低时延应用开发成本，屏蔽底层技术细节，帮助用户简化开发过程，缩短开发周期，极大的减少了开发和维护成本。

官网： [https://yomo.run](https://yomo.run/)

For English：https://github.com/yomorun/yomo

## QUIC

**QUIC** 的全称是 Quick UDP Internet Connections protocol, 由 Google 设计提出，目前由 IETF 工作组推动进展。其设计的目标是替代 TCP 成为 HTTP/3 的数据传输层协议。熹乐科技在物联网（IoT）和边缘计算（Edge Computing）场景也一直在打造底层基于 QUIC 通讯协议的边缘计算微服务框架 [YoMo](https://yomo.run)，长时间关注 QUIC 协议的发展，遂整理该文集并配以适当的中文翻译，方便更多关注 QUIC 协议的人学习。

## QUIC Weekly - 每周一草

在线社区：🍖[discord/quic](https://discord.gg/CTH3wv9)  
维护者：🦖[YoMo](https://yomo.run/)

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

### QUIC Weekly - 20201104期

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

### QUIC Weekly - 20201028期

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
  * IoT设备是应用QUIC协议的一个好场景，因为这些设备通常工作在无线（蜂窝）网络下（Cellular network），且需要快速建连、0-RTT和重传。但是，这些设备CPU能力普遍较弱。QUIC的作者其实多次提到QUIC对IoT应用场景有很大的提升，可惜的是，至今还没有一套为这个场景设计的协议栈（其实有啊：基于QUIC协议的Edge Computing框架: [🦖YoMo](https://yomo.run/)）
* **Article** [未来的Internet: HTTP/3 — No More TCP, let’s QUIC fix it（谐音梗我翻不出来了...）](https://thexbhpguy.medium.com/the-new-internet-http-3-no-more-tcp-lets-quic-fix-it-6a4cbb6280c7)

### QUIC Weekly - 20201021期

* 📢 QUIC 协议终于出现在 [IETF last call](https://mailarchive.ietf.org/arch/msg/ietf-announce/py1vC4Iuzq18Je4rwF69029oVOI/) 中。
* 📢 QUIC 草案32文件已出：
  * 运输：https://tools.ietf.org/html/draft-ietf-quic-transport-32
  * 恢复：https://tools.ietf.org/html/draft-ietf-quic-recovery-32
  * TLS：https://tools.ietf.org/html/draft-ietf-quic-tls-32
  * HTTP：https://tools.ietf.org/html/draft-ietf-quic-http-32
  * QPACK：https://tools.ietf.org/html/draft-ietf-quic-qpack-19
* **Adoption** 现在 Facebook 已经使用 #QUIC + ＃HTTP3 来处理其全球所有本机应用流量的75％以上！他们从新协议中看到了令人印象深刻的性能提升，尤其是在他们的视频流使用案例中。 [Facebook 如何将 QUIC 带给数十亿人](https://engineering.fb.com/networking-traffic/how-facebook-is-bringing-quic-to-billions/)
* **Adoption** [Node.js 15首次支持 QUIC 和 HTTP/3](https://www.infoworld.com/article/3586354/nodejs-15-debuts-support-for-http3-transport.html)。

### QUIC Weekly - 20201014期

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

### IETF进展

* [draft-ietf-quic-transport-32](https://datatracker.ietf.org/doc/draft-ietf-quic-transport/) QUIC: A UDP-Based Multiplexed and Secure Transport
* [draft-ietf-quic-tls-32](https://datatracker.ietf.org/doc/draft-ietf-quic-tls/) Using TLS to Secure QUIC
* [draft-ietf-quic-invariants-11](https://datatracker.ietf.org/doc/draft-ietf-quic-invariants/) Version-Independent Properties of QUIC
* [draft-ietf-quic-recovery-32](https://datatracker.ietf.org/doc/draft-ietf-quic-recovery/) QUIC Loss Detection and Congestion Control
* [draft-ietf-quic-version-negotiation-01](https://datatracker.ietf.org/doc/draft-ietf-quic-version-negotiation/) Compatible Version Negotiation for QUIC


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
  // yomo.RunDev(&EchoPlugin{}, "localhost:4241")
  yomo.RunDevWith(&EchoPlugin{}, "localhost:4241", yomo.OutputEchoData)
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
	return "0x11" //name
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
