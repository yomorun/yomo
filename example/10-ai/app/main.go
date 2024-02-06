package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"

	"github.com/yomorun/yomo"
	"golang.org/x/exp/slog"

	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/serverless"
)

var (
	addr       = "localhost:9000"
	sinkTag    = uint32(0x61)
	api        = "http://localhost:8000/azopenai/chat/completions"
	credential = "token:Happy New Year"
	peerID     = "user-1"
)

func main() {
	// sink
	sink := yomo.NewStreamFunction("sink", addr, yomo.WithSfnCredential(credential))
	sink.SetObserveDataTags(sinkTag)
	sink.SetHandler(func(ctx serverless.Context) {
		slog.Info("[sink] receive", "tag", sinkTag, "tagget", peerID, "data", string(ctx.Data()))
	})
	sink.SetWantedTarget(peerID)
	err := sink.Connect()
	if err != nil {
		slog.Error("[sink] connect", "err", err)
		os.Exit(1)
	}
	defer sink.Close()

	// app
	go requestInvokeAIFunction()
	sink.Wait()
}

func requestInvokeAIFunction() error {
	prompt := "What's the weather like in San Francisco, Melbourne, and Paris?"
	// invoke ai api
	userReq, err := json.Marshal(ai.ChatCompletionsRequest{
		PeerID: peerID,
		Prompt: prompt,
	})
	if err != nil {
		slog.Error("[app] ❌ Marshal data failure with err", "err", err)
		return err
	}
	req, err := http.NewRequest("POST", api, bytes.NewBuffer(userReq))
	if err != nil {
		slog.Error("[app] ❌ NewRequest failure with err", "err", err)
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	// set credential
	req.Header.Set("Authorization", "Bearer "+credential)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("[app] ❌ Invoke AI function failure with err", "err", err)
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("[app] ❌ ReadAll data failure with err", "err", err)
		return err
	}
	if resp.StatusCode != http.StatusOK {
		slog.Warn("[app] ⛔️Invoke AI function failure",
			"status", resp.StatusCode,
			"body", string(body),
		)
		return nil
	}
	slog.Info("[app] ✅ Invoke AI function", "prompt", prompt)

	return nil
}
