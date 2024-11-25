module llm-sfn-get-ip-and-latency

go 1.21

require (
	github.com/go-ping/ping v1.1.0
	github.com/yomorun/yomo v1.18.4
)

replace github.com/yomorun/yomo => ../../../

require (
	github.com/caarlos0/env/v6 v6.10.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/lmittmann/tint v1.0.4 // indirect
	github.com/sashabaranov/go-openai v1.35.6 // indirect
	golang.org/x/net v0.31.0 // indirect
	golang.org/x/sync v0.9.0 // indirect
	golang.org/x/sys v0.27.0 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
)
