# 优化和最佳实践

## 数据编码

**JSON**（**J**ava**S**cript **O**bject **N**otation, [/ˈdʒeɪsən/](https://zh.wikipedia.org/wiki/Help:英語國際音標)）是由[道格拉斯·克罗克福特](https://zh.wikipedia.org/wiki/道格拉斯·克羅克福特)构想和设计的一种轻量级[资料交换格式](https://zh.wikipedia.org/wiki/数据交换)。其内容由属性和值所组成，因此也有易于阅读和处理的优势。在示例中大量使用了 `JSON` 数据格式用于传输数据流，但在生产环境中`JSON`的编解码效率较低，尺寸也较大，为了保证流数据的高效传输，建议您在生产环境中使用 [Y3](https://github.com/yomorun/y3)，[MessagePack](https://msgpack.org/)，[ProtocolBuffers](https://developers.google.com/protocol-buffers/) 等高效二进制编码格式。

## 安全

`YoMo` 支持使用中央证书颁发机构对 `Zipper`，`Source`，`StreamFucntion` 之间的通信进行传输中加密。

`YoMo` 允许运营商和开发人员引入他们自己的证书，`scripts` 目录提供了证书生成脚本：

- generate_ca.sh
- generate_client.sh
- generate_server.sh

您可参照 [README.md](https://github.com/yomorun/yomo/blob/master/scripts/README.md) 文件说明，创建相关证书。

默认情况下，我们使用 `development` 开发模式，不进行服务端与客户端的双向 `TLS`认证，在生产环境下，**强烈建议**您修改如下环境变量：

- `YOMO_ENV`，将该值设置为 `production`
- `YOMO_TLS_CACERT_FILE`，CA 证书
- `YOMO_TLS_CERT_FILE`，证书
- `YOMO_TLS_KEY_FILE`，私钥

在 `Zipper`，`Source`，`StreamFucntion` 实例分别配置相应证书文件。

参考示例 [3-multi-sfn 运行设置](https://github.com/yomorun/yomo/blob/master/example/3-multi-sfn/Taskfile.yml) ，取消注释部分设置。

