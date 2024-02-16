package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/serverless"
)

type Parameter struct {
	TimeString     string `json:"timeString" jsonschema:"description=The time string to be converted"`
	SourceTimezone string `json:"sourceTimezone" jsonschema:"description=The source timezone of the time string, in IANA Time Zone Database identifier format"`
	TargetTimezone string `json:"targetTimezone" jsonschema:"description=The target timezone to convert the time string to, in IANA Time Zone Database identifier format"`
}

func Description() string {
	return "Extract time and timezone information from the following text. The desired format for the time is 'YYYY-MM-DD HH:MM:SS', and the timezone should be specified using an IANA Time Zone Database identifier."
}

func InputSchema() any {
	return &Parameter{}
}

const timeFormat = "2006-01-02 15:04:05"

func main() {
	sfn := yomo.NewStreamFunction(
		"fn-timezone-converter",
		"localhost:9000",
		yomo.WithSfnCredential("token:Happy New Year"),
		yomo.WithSfnAIFunctionDefinition(Description(), InputSchema()),
	)
	defer sfn.Close()

	sfn.SetObserveDataTags(0x10)

	// start
	err := sfn.Connect()
	if err != nil {
		slog.Error("[sfn] connect", "err", err)
		os.Exit(1)
	}

	sfn.SetHandler(handler)

	// set the error handler function when server error occurs
	sfn.SetErrorHandler(func(err error) {
		slog.Error("[sfn] receive server error", "err", err)
	})

	sfn.Wait()
}

func handler(ctx serverless.Context) {
	var msg Parameter
	err := json.Unmarshal(ctx.Data(), &msg)
	if err != nil {
		slog.Error("[sfn] json.Marshal error", "err", err)
		os.Exit(-2)
	}

	if msg.TargetTimezone == "" {
		msg.TargetTimezone = "UTC"
	}

	target, err := ConvertTimezone(msg.TimeString, msg.SourceTimezone, msg.TargetTimezone)
	if err != nil {
		slog.Error("[sfn] ConvertTimezone error", "err", err)
		return
	}

	ctx.WriteWithTarget(0x61, []byte(target), "user-1")
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
