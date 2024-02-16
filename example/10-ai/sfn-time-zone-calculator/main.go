package main

import (
	"fmt"
	"time"
)

const timeFormat = "2006-01-02 15:04:05"

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
