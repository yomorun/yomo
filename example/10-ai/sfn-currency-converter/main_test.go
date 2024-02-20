// Package yomo test main.s
package main

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetRates(t *testing.T) {
	targetCurrency := "AUD"

	cmd := exec.Command("bash", "-c", `cat usd.json | grep "AUD" | awk -F': ' '{print $2}' | sed 's/,//'`)
	output, err := cmd.Output()
	assert.NoError(t, err, "can not find jq command")

	outputStr := strings.TrimSpace(string(output)) // remove newline character
	expectedRate, err := strconv.ParseFloat(outputStr, 64)
	assert.NoError(t, err, "parse USD.json error")

	actualRate := getRates(targetCurrency)
	assert.EqualValues(t, actualRate, expectedRate)

	// if actualRate != expectedResult {
	// 	t.Errorf("Expected rate: %f, but got: %f", expectedRate, actualRate)
	// }
}
