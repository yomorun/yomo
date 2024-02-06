package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/yomorun/yomo"
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
	var msg Parameter
	err := json.Unmarshal(ctx.Data(), &msg)
	if err != nil {
		slog.Error("[sfn] json.Marshal error", "err", err)
		os.Exit(-2)
	}

	slog.Info("[sfn] << receive", "tag", 0x10, "data", fmt.Sprintf("target currency: %s, amount: %f", msg.Target, msg.Amount))

	// read all the target currency exchange rates from usd.json
	rate := getRates(msg.Target)
	if rate == 0 {
		err = ctx.WriteWithTarget(0x61, []byte("can not understand the target currency"), "user-1")
	} else {
		err = ctx.WriteWithTarget(0x61, []byte(fmt.Sprintf("The exchange rate of %s to USD is %f", msg.Target, rate)), "user-1")
	}

	if err != nil {
		slog.Error("[sfn] >> write error", "err", err)
	}
}

type Rates struct {
	Currency string  `json:"currency"`
	Rate     float64 `json:"rate"`
}

func getRates(targetCurrency string) float64 {
	file, err := os.Open("usd.json")
	if err != nil {
		fmt.Println(err)
		return 0
	}
	defer file.Close()

	rates := []Rates{}
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&rates)
	if err != nil {
		fmt.Println(err)
		return 0
	}

	for _, rate := range rates {
		if rate.Currency == targetCurrency {
			return rate.Rate
		}
		// fmt.Printf("Currency: %s, Rate: %f\n", rate.Currency, rate.Rate)
	}

	return 0
}
