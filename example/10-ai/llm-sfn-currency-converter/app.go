package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/yomorun/yomo/serverless"
)

type Parameter struct {
	SourceCurrency string  `json:"source" jsonschema:"description=The source currency to be queried in 3-letter ISO 4217 format"`
	TargetCurrency string  `json:"target" jsonschema:"description=The target currency to be queried in 3-letter ISO 4217 format"`
	Amount         float64 `json:"amount" jsonschema:"description=The amount of the currency to be converted to the target currency"`
}

func Description() string {
	return `if user asks currency exchange rate related questions, you should call this function. But if the source currency is other than USD (US Dollar), you should ignore calling tools.`
}

func InputSchema() any {
	return &Parameter{}
}

func Handler(ctx serverless.Context) {
	slog.Info("[sfn] receive", "ctx.data", string(ctx.Data()))

	var msg Parameter
	err := ctx.ReadLLMArguments(&msg)
	if err != nil {
		slog.Error("[sfn] json.Marshal error", "err", err)
		return
	}

	slog.Info("[sfn] << receive", "tag", 0x10, "data", fmt.Sprintf("%+v", msg))

	rate, err := fetchRate(msg.SourceCurrency, msg.TargetCurrency, msg.Amount)
	if err != nil {
		slog.Error("[sfn] >> fetchRate error", "err", err)
		return
	}

	result := fmt.Sprintf("based on today's exchange rate: %f, %f %s is equivalent to approximately %f %s", rate, msg.Amount, msg.SourceCurrency, msg.Amount*rate, msg.TargetCurrency)
	if rate == 0 {
		result = fmt.Sprintf("can not understand the target currency %s", msg.TargetCurrency)
	}

	err = ctx.WriteLLMResult(result)
	if err != nil {
		slog.Error("[sfn] >> write error", "err", err)
	}
}

type Rates struct {
	Rates map[string]float64 `json:"rates"`
}

func fetchRate(sourceCurrency string, targetCurrency string, _ float64) (float64, error) {
	resp, err := http.Get(fmt.Sprintf("https://openexchangerates.org/api/latest.json?app_id=%s&base=%s&symbols=%s", os.Getenv("API_KEY"), sourceCurrency, targetCurrency))
	if err != nil {
		return 0, err
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var rt *Rates
	err = json.Unmarshal(body, &rt)
	if err != nil {
		return 0, err
	}

	return getRates(targetCurrency, rt)
}

func getRates(targetCurrency string, rates *Rates) (float64, error) {
	if rates == nil {
		return 0, fmt.Errorf("can not get the target currency, target currency is %s", targetCurrency)
	}

	if rate, ok := rates.Rates[targetCurrency]; ok {
		return rate, nil
	}

	return 0, fmt.Errorf("can not get the target currency, target currency is %s", targetCurrency)
}
