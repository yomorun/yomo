package main

import (
	"fmt"
	"log/slog"
	"math/rand"
	"time"

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

// func Handler(aictx serverless.AIContext)
func Handler(ctx serverless.Context) {
	slog.Info("[sfn] << receive", "ctx.data", string(ctx.Data()))
	var msg Parameter
	err := ctx.ReadLLMArguments(&msg)
	if err != nil {
		slog.Error("[sfn] ReadLLMArguments error", "err", err)
		return
	}
	slog.Info("[sfn] << receive", "tag", tag, "msg", msg)
	data := fmt.Sprintf("[%s] temperature: %d°C", msg.CityName, rand.Intn(40))
	time.Sleep(time.Millisecond * 300)
	// helper ai function
	err = ctx.WriteLLMResult(data)
	if err == nil {
		slog.Info("[sfn] >> write", "tag", ai.ReducerTag, "msg", data)
		fnCall, err := ctx.LLMFunctionCall()
		if err != nil {
			slog.Error("[sfn] LLMFunctionCall error", "err", err)
			return
		}
		slog.Info("[sfn] >> write", "tag", ai.ReducerTag, "fnCall", fnCall)
	}
}

func DataTags() []uint32 {
	return []uint32{tag}
}
