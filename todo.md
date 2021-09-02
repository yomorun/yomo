## TODO

- [x] `source`/`stream function`，断线后，不能重新自动连接zipper
- [x] 注册 `stream function`后，没能即时删除
- [x] handshake 需要校验 `stream function` 的 `name/token` 是否有效
- [x] MetaFrame `Issuer()` 没有获取到值
- [x] 增加环境变量 `YOMO_LOG_LEVEL` 设置不同日志级别
- [x] 心跳 Ping/Pong（使用 quic-go 自带的 ping）
- [ ] 去除无用的 frame: Accepted/Rejected, Ping/Pong
- [ ] zipper 互通
- [ ] sfn 支持 `rx` 和 `raw bytes` 两种 Handler
- [ ] 添加/修改 frame，让传输的数据支持 scale

