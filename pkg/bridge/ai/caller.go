package ai

import (
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/ylog"
	"github.com/yomorun/yomo/serverless"
)

// Caller calls the invoke function and keeps the metadata and system prompt.
type Caller struct {
	CallSyncer
	source       yomo.Source
	reducer      yomo.StreamFunction
	md           metadata.M
	systemPrompt atomic.Value
	logger       *slog.Logger
}

// NewCaller returns a new caller.
func NewCaller(source yomo.Source, reducer yomo.StreamFunction, md metadata.M, callTimeout time.Duration) (*Caller, error) {
	logger := ylog.Default()

	reqCh, err := SourceWriteToChan(source, logger)
	if err != nil {
		return nil, err
	}

	resCh, err := ReduceToChan(reducer, logger)
	if err != nil {
		return nil, err
	}

	callSyncer := NewCallSyncer(logger, reqCh, resCh, callTimeout)

	caller := &Caller{
		CallSyncer: callSyncer,
		source:     source,
		reducer:    reducer,
		md:         md,
		logger:     logger,
	}

	return caller, nil
}

// SourceWriteToChan makes source write data to the channel.
// The TagFunctionCall objects are continuously be received from the channel and be sent by the source.
func SourceWriteToChan(source yomo.Source, logger *slog.Logger) (chan<- ai.FunctionCall, error) {
	err := source.Connect()
	if err != nil {
		return nil, err
	}

	ch := make(chan ai.FunctionCall)
	go func() {
		for c := range ch {
			buf, _ := c.Bytes()
			if err := source.WriteWithTarget(ai.FunctionCallTag, buf, c.FunctionName); err != nil {
				logger.Error("send data to zipper", "err", err.Error())
			}
		}
	}()

	return ch, nil
}

// ReduceToChan configures the reducer and returns a channel to accept messages from the reducer.
func ReduceToChan(reducer yomo.StreamFunction, logger *slog.Logger) (<-chan ToolCallResult, error) {
	reducer.SetObserveDataTags(ai.ReducerTag)

	messages := make(chan ToolCallResult)

	reducer.SetObserveDataTags(ai.ReducerTag)
	reducer.SetHandler(reduceFunc(messages, logger))

	if err := reducer.Connect(); err != nil {
		return nil, err
	}

	return messages, nil
}

func reduceFunc(messages chan ToolCallResult, logger *slog.Logger) core.AsyncHandler {
	return func(ctx serverless.Context) {
		invoke, err := ctx.LLMFunctionCall()
		if err != nil {
			messages <- ToolCallResult{ReqID: ""}
			logger.Error("parse function calling invoke", "err", err.Error())
			return
		}
		logger.Debug("sfn-reducer", "req_id", invoke.ReqID, "tool_call_id", invoke.ToolCallID, "result", string(invoke.Result))

		message := ToolCallResult{
			ReqID:        invoke.ReqID,
			FunctionName: invoke.FunctionName,
			ToolCallID:   invoke.ToolCallID,
			Content:      invoke.Result,
		}

		messages <- message
	}
}

type promptOperation struct {
	prompt    string
	operation SystemPromptOp
}

// SystemPromptOp defines the operation of system prompt
type SystemPromptOp int

const (
	SystemPromptOpOverwrite SystemPromptOp = 0
	SystemPromptOpDisabled  SystemPromptOp = 1
	SystemPromptOpPrefix    SystemPromptOp = 2
)

// SetSystemPrompt sets the system prompt
func (c *Caller) SetSystemPrompt(prompt string, op SystemPromptOp) {
	p := &promptOperation{
		prompt:    prompt,
		operation: op,
	}
	c.systemPrompt.Store(p)
}

// GetSystemPrompt gets the system prompt
func (c *Caller) GetSystemPrompt() (prompt string, op SystemPromptOp) {
	if v := c.systemPrompt.Load(); v != nil {
		pop := v.(*promptOperation)
		return pop.prompt, pop.operation
	}
	return "", SystemPromptOpOverwrite
}

// Metadata returns the metadata of caller.
func (c *Caller) Metadata() metadata.M {
	return c.md
}

// Close closes the caller.
func (c *Caller) Close() error {
	_ = c.CallSyncer.Close()

	var err error
	if err = c.source.Close(); err != nil {
		c.logger.Error("callSyncer writer close", "err", err.Error())
	}

	if err = c.reducer.Close(); err != nil {
		c.logger.Error("callSyncer reducer close", "err", err.Error())
	}

	return err
}
