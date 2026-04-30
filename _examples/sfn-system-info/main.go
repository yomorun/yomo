package main

import (
	"fmt"
	"log/slog"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/yomorun/yomo/serverless"
)

// Init is an optional function invoked during the initialization phase of the
// sfn instance.
func Init() error {
	return nil
}

// Description outlines the functionality for the LLM Function Calling feature.
func Description() string {
	return `Get the system resource information of the edge node, 
	including CPU usage, memory usage, and OS information. 
	This is useful for monitoring the health and load of geo-distributed edge nodes.`
}

// InputSchema defines the argument structure for LLM Function Calling.
func InputSchema() any {
	return &LLMArguments{}
}

// LLMArguments defines the arguments for the LLM Function Calling.
type LLMArguments struct {
	IncludeCPU    bool `json:"include_cpu" jsonschema:"description=Whether to include CPU usage information,default=true"`
	IncludeMemory bool `json:"include_memory" jsonschema:"description=Whether to include memory usage information,default=true"`
}

// Handler orchestrates the core processing logic of this function.
func Handler(ctx serverless.Context) {
	var p LLMArguments
	// deserialize the arguments from llm tool_call response
	ctx.ReadLLMArguments(&p)

	res := "System Information:\n"
	res += fmt.Sprintf("- OS: %s\n", runtime.GOOS)
	res += fmt.Sprintf("- Arch: %s\n", runtime.GOARCH)

	if p.IncludeCPU {
		percent, err := cpu.Percent(time.Second, false)
		if err == nil && len(percent) > 0 {
			res += fmt.Sprintf("- CPU Usage: %.2f%%\n", percent[0])
		}
	}

	if p.IncludeMemory {
		v, err := mem.VirtualMemory()
		if err == nil {
			res += fmt.Sprintf("- Memory Usage: %.2f%% (Used: %v MB, Total: %v MB)\n", 
				v.UsedPercent, v.Used/1024/1024, v.Total/1024/1024)
		}
	}

	// return the result back to LLM
	ctx.WriteLLMResult(res)

	slog.Info("system-info", "include_cpu", p.IncludeCPU, "include_memory", p.IncludeMemory, "result", res)
}
