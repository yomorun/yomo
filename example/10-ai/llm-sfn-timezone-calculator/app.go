package main

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/yomorun/yomo/serverless"
)

type Parameter struct {
	TimeString     string `json:"timeString" jsonschema:"description=The source time string to be converted, the desired format is 'YYYY-MM-DD HH:MM:SS'"`
	SourceTimezone string `json:"sourceTimezone" jsonschema:"description=The source timezone of the time string, in IANA Time Zone Database identifier format"`
	TargetTimezone string `json:"targetTimezone" jsonschema:"description=The target timezone to convert the timeString to, in IANA Time Zone Database identifier format"`
}

func Description() string {
	return `if user asks timezone converter related questions, extract the source time and timezone information to "timeString" and "sourceTimezone", extract the target timezone information to "targetTimezone". the desired "timeString" format is "YYYY-MM-DD HH:MM:SS". the "sourceTimezone" and "targetTimezone" are in IANA Time Zone Database identifier format. The function will convert the time from the source timezone to the target timezone and return the converted time as a string in the format "YYYY-MM-DD HH:MM:SS". If you are not sure about the date value of "timeString", you pretend date as today.`
}

func InputSchema() any {
	return &Parameter{}
}

const timeFormat = "2006-01-02 15:04:05"

func Handler(ctx serverless.Context) {
	slog.Info("[sfn] receive", "ctx.data", string(ctx.Data()))

	var msg Parameter
	err := ctx.ReadLLMArguments(&msg)
	if err != nil {
		slog.Error("[sfn] json.Marshal error", "err", err)
		return
	}

	if msg.TargetTimezone == "" {
		msg.TargetTimezone = "UTC"
	}

	// should gurantee date will not be "YYYY-MM-DD"
	if strings.Contains(msg.TimeString, "YYYY-MM-DD") {
		msg.TimeString = strings.ReplaceAll(msg.TimeString, "YYYY-MM-DD", time.Now().Format("2006-01-02"))
	}

	targetTime, err := ConvertTimezone(msg.TimeString, msg.SourceTimezone, msg.TargetTimezone)
	if err != nil {
		slog.Error("[sfn] ConvertTimezone error", "err", err)
		return
	}

	slog.Info("[sfn]", "result", targetTime)

	val := fmt.Sprintf("This time in timezone %s is %s when %s in %s", msg.TargetTimezone, targetTime, msg.TimeString, msg.SourceTimezone)

	ctx.WriteLLMResult(val)
}

func DataTags() []uint32 {
	return []uint32{0x12}
}

// ConvertTimezone converts the current time from the source timezone to the target timezone.
// It returns the converted time as a string in the format "2006-01-02 15:04:05".
func ConvertTimezone(timeString, sourceTimezone, targetTimezone string) (string, error) {
	// Get the location of the source timezone
	sourceLoc, err := time.LoadLocation(sourceTimezone)
	if err != nil {
		return "", fmt.Errorf("invalid source timezone: %v", err)
	}

	// Get the time in the source timezone
	sourceTime, err := time.ParseInLocation(timeFormat, timeString, sourceLoc)
	if err != nil {
		return "", fmt.Errorf("invalid time string: %v", err)
	}

	// Get the location of the target timezone
	targetLoc, err := time.LoadLocation(targetTimezone)
	if err != nil {
		return "", fmt.Errorf("invalid target timezone: %v", err)
	}

	// Convert the source time to the target timezone
	targetTime := sourceTime.In(targetLoc)

	// Return the target time as a string
	return targetTime.Format(timeFormat), nil
}
