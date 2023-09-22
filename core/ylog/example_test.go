package ylog_test

import (
	"io"
	"net"

	"github.com/yomorun/yomo/core/ylog"
)

func Example() {
	logger := ylog.NewFromConfig(ylog.Config{
		Level:       "warn",
		Format:      "json",
		ErrorOutput: "stdout",
		DisableTime: true,
	})

	ylog.SetDefault(logger.With("hello", "yomo").WithGroup("syslog"))

	ylog.Debug("debug", "aaa", "bbb")
	ylog.Info("info", "ccc", "ddd")
	ylog.Warn("warn", "eee", "fff")
	ylog.Error("error", "err", io.EOF, "eee", "fff")

	sysLogger := ylog.NewFromConfig(ylog.Config{
		Level:       "error",
		Format:      "json",
		ErrorOutput: "stdout",
		DisableTime: true,
	})

	sysLogger = sysLogger.WithGroup("syslog")

	sysLogger.Error("sys error", "err", net.ErrClosed, "ggg", "hhh")

	// Output:
	// {"level":"WARN","msg":"warn","hello":"yomo","syslog":{"eee":"fff"}}
	// {"level":"ERROR","msg":"error","hello":"yomo","syslog":{"err":"EOF","eee":"fff"}}
	// {"level":"ERROR","msg":"sys error","syslog":{"err":"use of closed network connection","ggg":"hhh"}}
}
