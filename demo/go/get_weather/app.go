package main

import (
	"fmt"
	"io"
	"net/http"
)

const Description = "Get weather for a city"

type Arguments struct {
	City string `json:"city" jsonschema:"description=The city name to get the weather for"`
}

type Result string

func Handler(args Arguments) (Result, error) {
	fmt.Println("start to query weather for city:", args.City)

	url := fmt.Sprintf("https://wttr.in/%s?format=3", args.City)
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to query weather, status code: %d", resp.StatusCode)
	}

	result, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return Result(result), nil
}
