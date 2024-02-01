package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"

	"github.com/yomorun/yomo"
	"golang.org/x/exp/slog"

	"github.com/yomorun/yomo/core/ai"
	"github.com/yomorun/yomo/serverless"
)

var (
	addr        = "localhost:9000"
	name        = "get-weather"
	tag         = uint32(0x60)
	description = "Get the current weather for `city_name`"
	api         = "http://localhost:8000/azopenai/chat/completions"
)

func main() {
	// source
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

	// sink
	sink := yomo.NewStreamFunction("sink", "localhost:9000")
	sink.SetObserveDataTags(0x61)
	sink.SetHandler(func(ctx serverless.Context) {
		slog.Info("[sink] receive data", "data", string(ctx.Data()))
	})
	err = sink.Connect()
	if err != nil {
		slog.Error("[sink] connect", "err", err)
		os.Exit(1)
	}
	defer sink.Close()

	// app
	go requestInvokeAIFunction(source)
	sink.Wait()
}

func requestInvokeAIFunction(source yomo.Source) error {
	prompt := "What's the weather like in San Francisco, Melbourne, and Paris?"
	// invoke ai api
	req, _ := json.Marshal(ai.ChatCompletionsRequest{
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
