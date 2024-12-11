module llm-sfn-get-ip-and-latency

go 1.23

toolchain go1.23.0

require (
	github.com/go-ping/ping v1.1.0
	github.com/yomorun/yomo v1.18.4
)

replace github.com/yomorun/yomo => ../../../

require (
	github.com/caarlos0/env/v6 v6.10.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/lmittmann/tint v1.0.4 // indirect
	github.com/sashabaranov/go-openai v1.35.7 // indirect
	golang.org/x/net v0.32.0 // indirect
	golang.org/x/sync v0.10.0 // indirect
	golang.org/x/sys v0.28.0 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
)
