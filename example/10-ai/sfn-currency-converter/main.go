package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/serverless"
	"golang.org/x/exp/slog"
)

type Parameter struct {
	SourceCurrency string  `json:"source" jsonschema:"description=The source currency to be queried in 3-letter ISO 4217 format"`
	TargetCurrency string  `json:"target" jsonschema:"description=The target currency to be queried in 3-letter ISO 4217 format"`
	Amount         float64 `json:"amount" jsonschema:"description=The amount of the USD currency to be converted to the target currency"`
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

	fcCtx, err := ai.ParseFunctionCallContext(ctx)
	if err != nil {
		slog.Error("[sfn] NewFunctionCallingParameters error", "err", err)
		return
	}

	var msg Parameter
	err = fcCtx.UnmarshalArguments(&msg)
	if err != nil {
		slog.Error("[sfn] json.Marshal error", "err", err)
		return
	}

	slog.Info("[sfn] << receive", "tag", 0x10, "data", fmt.Sprintf("%+v", msg))

	// read all the target currency exchange rates from usd.json
	// rate := getRates(msg.Target)
	rate, err := fetchRate(msg.SourceCurrency, msg.TargetCurrency, msg.Amount)
	if err != nil {
		slog.Error("[sfn] >> fetchRate error", "err", err)
		fcCtx.WriteErrors(err)
		return
	}

	result := ""
	if rate == 0 {
		result = fmt.Sprintf("can not understand the target currency, target currency is %s", msg.TargetCurrency)
	} else {
		result = fmt.Sprintf("%f", msg.Amount*rate)
	}

	// err = ctx.Write(invoke.CreatePayload(result))
	fcCtx.SetRetrievalResult(fmt.Sprintf("based on today's exchange rate: %f, %f %s is equivalent to approximately %f %s", rate, msg.Amount, msg.SourceCurrency, msg.Amount*rate, msg.TargetCurrency))
	err = fcCtx.Write(result)

	if err != nil {
		slog.Error("[sfn] >> write error", "err", err)
	}
}

func init() {
	// read API_KEY from .env
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
		os.Exit(-1)
	}
}

type Rates struct {
	Rates map[string]float64 `json:"rates"`
}

func fetchRate(sourceCurrency string, targetCurrency string, amount float64) (float64, error) {
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
