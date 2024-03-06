package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/serverless"
)

type Parameter struct {
	TimeString     string `json:"timeString" jsonschema:"description=The source time string to be converted, the desired format is 'YYYY-MM-DD HH:MM:SS'"`
	SourceTimezone string `json:"sourceTimezone" jsonschema:"description=The source timezone of the time string, in IANA Time Zone Database identifier format"`
	TargetTimezone string `json:"targetTimezone" jsonschema:"description=The target timezone to convert the timeString to, in IANA Time Zone Database identifier format"`
}

func Description() string {
	return "Extract the source time and timezone information to `timeString` and `sourceTimezone`, extract the target timezone information to `targetTimezone`. the desired `timeString` format is `YYYY-MM-DD HH:MM:SS`. the `sourceTimezone` and `targetTimezone` are in IANA Time Zone Database identifier format. The function will convert the time from the source timezone to the target timezone and return the converted time as a string in the format `YYYY-MM-DD HH:MM:SS`. If you are not sure about the date value of `timeString`, set date value to `1900-01-01`"
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

	sfn.SetObserveDataTags(0x12)

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
	slog.Info("[sfn] receive", "ctx.data", string(ctx.Data()))

	fcCtx, err := ai.ParseFunctionCallContext(ctx)
	if err != nil {
		slog.Error("[sfn] NewFunctionCallingParameters error", "err", err)
		return
	}

	var msg Parameter
	err = fcCtx.UnmarshalArguments(&msg)
	if err != nil {
		slog.Error("[sfn] json.Marshal error", "err", err)
		return
	}

	if msg.TargetTimezone == "" {
		msg.TargetTimezone = "UTC"
	}

	targetTime, err := ConvertTimezone(msg.TimeString, msg.SourceTimezone, msg.TargetTimezone)
	if err != nil {
		slog.Error("[sfn] ConvertTimezone error", "err", err)
		fcCtx.WriteErrors(err)
		return
	}

	slog.Info("[sfn] result", "result", targetTime)

	fcCtx.SetRetrievalResult(fmt.Sprintf("The time in timezone %s is %s", msg.TargetTimezone, targetTime))

	fcCtx.Write(targetTime)
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
