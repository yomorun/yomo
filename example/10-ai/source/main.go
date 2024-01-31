package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"

	"github.com/yomorun/yomo"
	"golang.org/x/exp/slog"

	// TODO: need dynamic load
	// _ "github.com/yomorun/yomo/pkg/ai/azopenai"
	"github.com/yomorun/yomo/core/ai"
)

var (
	addr        = "localhost:9000"
	appID       = ""
	name        = "get-weather"
	tag         = uint32(0x60)
	description = "Get the current weather for `city_name`"
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
	// invoke ai api
	api := "http://localhost:8000/azopenai/chat/completions"
	req, _ := json.Marshal(ai.ChatCompletionsRequest{
		AppID:  appID,
		Tag:    tag,
		Prompt: prompt,
	})
	// TODO: add bearer token, it's credential
	// req.Header.Set("Authorization", "Bearer "+cred)
	resp, err := http.Post(api, "application/json", bytes.NewBuffer(req))
	if err != nil {
		slog.Error("[source] ❌ Invoke AI function failure with err", "err", err)
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("[source] ❌ ReadAll data failure with err", "err", err)
		return err
	}
	if resp.StatusCode != http.StatusOK {
		slog.Warn("[source] ⛔️Invoke AI function failure", "body", string(body))
		return nil
	}
	slog.Info("[source] ✅ Invoke AI function", "body", string(body))
	// send data to YoMo-Zipper
	var chatCompletionsResponse ai.ChatCompletionsResponse
	err = json.Unmarshal(body, &chatCompletionsResponse)
	if err != nil {
		slog.Error("[source] ❌ Unmarshal data failure with err", "err", err)
		return err
	}
	for _, fd := range chatCompletionsResponse.Functions {
		slog.Info("[source] 🅰️ Invoke AI function", "functions", fd.Name, "arguments", fd.Arguments)
		err := source.Write(tag, []byte(fd.Arguments))
		if err != nil {
			slog.Error("[source] ❌ Emit to YoMo-Zipper failure with err", "err", err)
			return err

		} else {
			slog.Info("[source] ✅ Emit to YoMo-Zipper")
		}
	}

	return nil
}
