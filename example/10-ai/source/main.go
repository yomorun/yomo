package main

import (
	"encoding/json"
	"os"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/core/ai"
	"golang.org/x/exp/slog"

	// TODO: need dynamic load
	_ "github.com/yomorun/yomo/pkg/ai/azopenai"
)

var (
	addr        = "localhost:9000"
	tag  uint32 = 0x60
)

// TEST: need delete
type Msg struct {
	CityName string `json:"city_name" jsonschema:"description=The name of the city to be queried"`
}

func main() {
	// connect to YoMo-Zipper.
	if v := os.Getenv("YOMO_ADDR"); v != "" {
		addr = v
	}
	source := yomo.NewSource("yomo-source", addr, yomo.WithSourceReConnect())
	err := source.Connect()
	if err != nil {
		slog.Error("[source] ❌ Emit the data to YoMo-Zipper failure with err", "err", err)
		return
	}

	defer source.Close()

	// set the error handler function when server error occurs
	source.SetErrorHandler(func(err error) {
		slog.Error("[source] receive server error", "err", err)
		os.Exit(1)
	})

	err = requestInvokeAIFunction(source)
	slog.Error("[source] >>>> ERR", "err", err)
	// TODO: sink
	select {}
}

func requestInvokeAIFunction(source yomo.Source) error {
	prompt := "What's the weather like in San Francisco, Melbourne, and Paris?"
	// register ai function
	err := ai.RegisterFunctionCaller("appID", tag, "chatCompletionFunction", "chatCompletionFunction", &Msg{})
	if err != nil {
		slog.Error("[source] ❌ Register AI function failure with err", "err", err)
		return err
	}
	// invoke ai api
	resp, err := ai.GetChatCompletions("appID", tag, prompt)
	if err != nil {
		slog.Error("[source] ❌ Invoke AI function failure with err", "err", err)
		return err
	}
	slog.Info("[source] ✅ Invoke AI function", "resp", resp)
	msg := Msg{
		CityName: "San Francisco",
	}
	data, err := json.Marshal(msg)
	if err != nil {
		slog.Error("[source] ❌ Marshal data failure with err", "err", err)
		return err
	}
	// send data to YoMo-Zipper
	err = source.Write(tag, data)
	if err != nil {
		slog.Error("[source] ❌ Emit to YoMo-Zipper failure with err", "err", err, "data", data)
		return err

	} else {
		slog.Info("[source] ✅ Emit to YoMo-Zipper", "data", string(data))
	}
	return nil
}
