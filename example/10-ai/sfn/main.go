package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/core/ai"
	"github.com/yomorun/yomo/serverless"
	"golang.org/x/exp/slog"

	// TODO: need dynamic load
	_ "github.com/yomorun/yomo/pkg/ai/azopenai"
)

var (
	name           = "get-weather"
	addr           = "localhost:9000"
	tag     uint32 = 0x60
	sinkTag uint32 = 0x61
)

type Msg struct {
	CityName string `json:"city_name" jsonschema:"description=The name of the city to be queried"`
}

// ================== AI Required ==================
func Description() string {
	return "Get the current weather for `city_name`"
}

func InputSchema() any {
	return &Msg{}
}

// ================== AI End ==================

func main() {
	sfn := yomo.NewStreamFunction(name, addr)
	sfn.SetObserveDataTags(tag)
	defer sfn.Close()

	// TODO: 检查是实现 AI FunctionCaller
	// ai function caller
	// if llmfn, ok := any(sfn).(llm.LLMFunctionCaller); ok {
	// 	// sfn.SetAppID(appID)
	// 	llmfn.SetDescription("get weather")
	// 	llmfn.SetModel(&UserModel{})
	// 	if err := llm.Register(llmfn); err != nil {
	// 		slog.Error("llm register", "error", err)
	// 		return
	// 	}
	// }

	// set handler
	sfn.SetHandler(handler)
	// start
	err := sfn.Connect()
	if err != nil {
		slog.Error("[sfn] connect", "err", err)
		os.Exit(1)
	}

	description := serverless.Description()
	slog.Info("[sfn] description", "description", description)
	// TODO: 如何获取 appID?
	appID := "appID"
	// TODO: 注册 AI Function
	err = ai.RegisterFunctionCaller(appID, tag, name, Description(), InputSchema())
	if err != nil {
		slog.Error("[sfn] register ai function caller", "err", err)
		os.Exit(-1)
	}
	// set the error handler function when server error occurs
	sfn.SetErrorHandler(func(err error) {
		slog.Error("[sfn] receive server error", "err", err)
	})

	sfn.Wait()
}

func handler(ctx serverless.Context) {
	var msg Msg
	err := json.Unmarshal(ctx.Data(), &msg)
	if err != nil {
		slog.Error("[sfn] json.Marshal error", "err", err)
		os.Exit(-2)
	} else {
		slog.Info("[sfn]", "got", 0x60, "data", msg)
		data := fmt.Sprintf("[%s] temperature: %d°C", msg.CityName, rand.Intn(40))
		err = ctx.Write(sinkTag, []byte(data))
		if err == nil {
			slog.Info("[sfn] write", "tag", sinkTag, "data", data)
		}
	}
}
