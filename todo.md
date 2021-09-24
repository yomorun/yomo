## TODO

- [x] `source`/`stream function`，断线后，不能重新自动连接 zipper
- [x] 注册 `stream function`后，没能即时删除
- [x] `stream function` 的删除代码是否可以简化合并
- [x] handshake 需要校验 `stream function` 的 `name/token` 是否有效
- [x] MetaFrame `Issuer()` 没有获取到值
- [x] 增加环境变量 `YOMO_LOG_LEVEL` 设置不同日志级别
- [x] 心跳 Ping/Pong（使用 quic-go 自带的 ping）
- [x] 去除无用的 frame: Accepted/Rejected, Ping/Pong
  - [ ] Long term: 考虑 Client->Server 使用 2 个 stream，区分控制信令传输和数据传输。传输大文件时该功能会好用。
- [x] 多 sfn 支持
- [x] zipper 互通
- [x] sfn 支持 `rx` 和 `raw bytes` 两种 Handler
- [ ] 添加/修改 frame，让传输的数据支持 scale
- [x] yomo-aftership POC 测试(目前测试结果没有明显改进)
- [x] YoMo 链路跟踪
- [x] ConnectionType 应该修改为 ClientType
- [x] 命名应该规范化,明确表示意图
