package main

import (
	"fmt"
	"log/slog"
	"math/rand"
	"os"

	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/serverless"
)

var tag uint32 = 0x11

type Parameter struct {
	CityName string `json:"city_name" jsonschema:"description=The name of the city to be queried"`
}

// Description returns the description of this AI function.
func Description() string {
	return "Get the current weather for `city_name`"
}

// InputSchema returns the input schema of this AI function.
func InputSchema() any {
	return &Parameter{}
}

func Handler(ctx serverless.Context) {
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
		err = fcCtx.Write(data)
		if err == nil {
			slog.Info("[sfn] >> write", "tag", ai.ReducerTag, "data", data)
		}
	}
}

func DataTags() []uint32 {
	return []uint32{tag}
}
