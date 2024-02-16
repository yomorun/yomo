package main

import "testing"

func TestConvertTimezone(t *testing.T) {
	tests := []struct {
		name           string
		timeString     string
		sourceTimezone string
		targetTimezone string
		wantErr        bool
		targetTime     string
	}{
		{
			name:           "valid timezones and time string",
			timeString:     "2023-02-16 00:00:00",
			sourceTimezone: "America/New_York",
			targetTimezone: "Asia/Singapore",
			wantErr:        false,
			targetTime:     "2023-02-16 13:00:00",
		},
		{
			name:           "valid timezones and time string",
			timeString:     "2024-02-15 17:00:00",
			sourceTimezone: "America/Los_Angeles",
			targetTimezone: "Europe/Berlin",
			wantErr:        false,
			targetTime:     "2024-02-16 02:00:00",
		},
		{
			name:           "invalid time string",
			timeString:     "invalid time string",
			sourceTimezone: "America/New_York",
			targetTimezone: "Asia/Shanghai",
			wantErr:        true,
		},
		{
			name:           "invalid source timezone",
			timeString:     "2022-01-01 12:00:00",
			sourceTimezone: "Invalid/Timezone",
			targetTimezone: "Asia/Shanghai",
			wantErr:        true,
		},
		{
			name:           "invalid target timezone",
			timeString:     "2022-01-01 12:00:00",
			sourceTimezone: "America/New_York",
			targetTimezone: "Invalid/Timezone",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, err := ConvertTimezone(tt.timeString, tt.sourceTimezone, tt.targetTimezone)
			if (err != nil) != tt.wantErr {
				t.Errorf("ConvertTimezone() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if target != tt.targetTime {
				t.Errorf("ConvertTimezone() target = %v, want %v", target, tt.targetTime)
			}
		})
	}
}
