package main

import (
	"fmt"
	"log/slog"
	"net"
	"os"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/serverless"

	"github.com/go-ping/ping"
)

type Parameter struct {
	Domain string `json:"domain" jsonschema:"description=Domain of the website,example=example.com"`
}

func Description() string {
	return `if user asks ip or network latency of a domain, you should return the result of the giving domain. try your best to dissect user expressions to infer the right domain names`
}

func InputSchema() any {
	return &Parameter{}
}

func main() {
	sfn := yomo.NewStreamFunction(
		"fn-exchange-rates",
		"localhost:9000",
		yomo.WithSfnCredential("token:Happy New Year"),
		yomo.WithSfnAIFunctionDefinition(Description(), InputSchema()),
	)
	defer sfn.Close()

	sfn.SetObserveDataTags(0x10)

	// start
	err := sfn.Connect()
	if err != nil {
		slog.Error("[sfn] connect", "err", err)
		os.Exit(1)
	}

	sfn.SetHandler(handler)

	// set the error handler function when server error occurs
	sfn.SetErrorHandler(func(err error) {
		slog.Error("[sfn] receive server error", "err", err)
	})

	sfn.Wait()
}

func handler(ctx serverless.Context) {
	fc, err := ai.ParseFunctionCallContext(ctx)
	if err != nil {
		slog.Error("[sfn] parse function call context", "err", err)
		return
	}

	var msg Parameter
	err = fc.UnmarshalArguments(&msg)
	if err != nil {
		slog.Error("[sfn] unmarshal arguments", "err", err)
		return
	}

	if msg.Domain == "" {
		slog.Warn("[sfn] domain is empty")
		return
	}

	slog.Info("*fired*", "domain", msg.Domain)

	// get ip of the domain
	ips, err := net.LookupIP(msg.Domain)
	if err != nil {
		slog.Error("[sfn] could not get IPs", "err", err)
		return
	}

	for _, ip := range ips {
		slog.Info("[sfn] get ip", "domain", msg.Domain, "ip", ip)
	}

	// get ip[0] ping latency
	// get ip[0] ping latency
	pinger, err := ping.NewPinger(ips[0].String())
	if err != nil {
		slog.Error("[sfn] could not create pinger", "err", err)
		return
	}

	pinger.Count = 3
	pinger.Run()                 // blocks until finished
	stats := pinger.Statistics() // get send/receive/rtt stats

	slog.Info("[sfn] get ping latency", "domain", msg.Domain, "ip", ips[0], "latency", stats.AvgRtt)

	fc.SetRetrievalResult(fmt.Sprintf("domain %s has ip %s with average latency %s", msg.Domain, ips[0], stats.AvgRtt))
	fc.Write(ips[0].String())
}
