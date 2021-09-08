## TODO

- [x] `source`/`stream function`，断线后，不能重新自动连接zipper
- [x] 注册 `stream function`后，没能即时删除
- [ ] `stream function` 的删除代码是否可以简化合并
- [x] handshake 需要校验 `stream function` 的 `name/token` 是否有效
- [x] MetaFrame `Issuer()` 没有获取到值
- [x] 增加环境变量 `YOMO_LOG_LEVEL` 设置不同日志级别
- [x] 心跳 Ping/Pong（使用 quic-go 自带的 ping）
- [ ] 去除无用的 frame: Accepted/Rejected, Ping/Pong
- [ ] zipper 互通
- [ ] sfn 支持 `rx` 和 `raw bytes` 两种 Handler
- [ ] 添加/修改 frame，让传输的数据支持 scale
- [x] yomo-aftership POC 测试(目前测试结果没有明显改进)
- [x] YoMo 链路跟踪
- [ ] ConnectionType 应该修改为 ClientType
- [ ] 命名应该规范化,明确表示意图
```go
	funcs              *ConcurrentMap // 服务器收到函数请求
	funcBuckets        map[int]string // 用户配置的流处理函数
	connSfnMap         sync.Map // 连接与函数的对应关系
```

