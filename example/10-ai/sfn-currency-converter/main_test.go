// Package yomo test main.s
package main

import (
	"testing"
)

func TestGetRates(t *testing.T) {
	targetCurrency := "AUD"
	expectedRate := 1.0
	expectedResult := 1.538269

	actualRate := getRates(targetCurrency)

	if actualRate != expectedResult {
		t.Errorf("Expected rate: %f, but got: %f", expectedRate, actualRate)
	}
}
