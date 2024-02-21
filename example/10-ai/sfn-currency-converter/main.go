package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/serverless"
	"golang.org/x/exp/slog"
)

type Parameter struct {
	Target string  `json:"target" jsonschema:"description=The target currency to be queried in 3-letter ISO 4217 format"`
	Amount float64 `json:"amount" jsonschema:"description=The amount of the USD currency to be converted to the target currency"`
}

func Description() string {
	return "Get the current exchange rates"
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
	slog.Info("[sfn] receive", "ctx.data", string(ctx.Data()))

	invoke, err := ai.NewFunctionCallingInvoke(ctx)
	if err != nil {
		slog.Error("[sfn] NewFunctionCallingParameters error", "err", err)
		return
	}

	var msg Parameter
	err = json.Unmarshal([]byte(invoke.Arguments), &msg)
	if err != nil {
		slog.Error("[sfn] json.Marshal error", "err", err)
		return
	}

	slog.Info("[sfn] << receive", "tag", 0x10, "data", fmt.Sprintf("target currency: %s, amount: %f", msg.Target, msg.Amount))

	// read all the target currency exchange rates from usd.json
	rate := getRates(msg.Target)
	result := ""
	if rate == 0 {
		result = fmt.Sprintf("can not understand the target currency, target currency is %s", msg.Target)
	} else {
		result = fmt.Sprintf("%f", msg.Amount*rate)
	}

	err = ctx.Write(invoke.CreatePayload(result))

	if err != nil {
		slog.Error("[sfn] >> write error", "err", err)
	}
}

type Rates struct {
	Rates map[string]float64 `json:"rates"`
}

var rates *Rates

func init() {
	file, err := os.Open("usd.json")
	if err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
	defer file.Close()

	byteValue, _ := io.ReadAll(file)

	json.Unmarshal(byteValue, &rates)
}

func getRates(targetCurrency string) float64 {
	if rates == nil {
		return 0
	}

	if rate, ok := rates.Rates[targetCurrency]; ok {
		return rate
	}

	return 0
}
