package main

import (
	"fmt"
	"log/slog"
	"math/rand"
	"os"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/serverless"
)

var (
	name              = "get-weather"
	addr              = "localhost:9000"
	tag        uint32 = 0x11
	credential        = "token:Happy New Year"
)

type Parameter struct {
	CityName string `json:"city_name" jsonschema:"description=The name of the city to be queried"`
}

// ================== AI Required ==================
// Description returns the description of this AI function.
func Description() string {
	return "Get the current weather for `city_name`"
}

// InputSchema returns the input schema of this AI function.
func InputSchema() any {
	return &Parameter{}
}

// ================== main ==================

func main() {
	sfn := yomo.NewStreamFunction(
		name,
		addr,
		yomo.WithSfnCredential(credential),
		yomo.WithSfnAIFunctionDefinition(Description(), InputSchema()),
	)
	sfn.SetObserveDataTags(tag)
	defer sfn.Close()

	// set handler
	sfn.SetHandler(handler)

	// sfn.SetWantedTarget(peerID)

	// start
	err := sfn.Connect()
	if err != nil {
		slog.Error("[sfn] connect", "err", err)
		os.Exit(1)
	}

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
		os.Exit(-2)
	} else {
		slog.Info("[sfn] << receive", "tag", tag, "data", msg)
		data := fmt.Sprintf("[%s] temperature: %d°C", msg.CityName, rand.Intn(40))
		fcCtx.SetRetrievalResult(data)
		err = fcCtx.Write(fmt.Sprintf("%d", rand.Intn(40)))
		if err == nil {
			slog.Info("[sfn] >> write", "tag", ai.ReducerTag, "data", data)
		}
	}
}
