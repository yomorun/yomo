// Package yomo test main.s
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetRates(t *testing.T) {
	var rates *Rates
	file, err := os.Open("usd.json")
	if err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
	defer file.Close()
	byteValue, _ := io.ReadAll(file)
	json.Unmarshal(byteValue, &rates)

	targetCurrency := "AUD"
	expectedRate := 1.531309

	actualRate, err := getRates(targetCurrency, rates)
	assert.InEpsilon(t, actualRate, expectedRate, 1e-6)
	assert.NoError(t, err, "getRates error")
}
