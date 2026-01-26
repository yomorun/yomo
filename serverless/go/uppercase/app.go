package main

import (
	"fmt"
	"strings"
)

type Arguments string

type Result string

func Handler(args Arguments) (Result, error) {
	fmt.Println("args:", args)

	if len(args) > 20 {
		return "", fmt.Errorf("input '%s' is too long", args)
	}

	result := strings.ToUpper(string(args))
	fmt.Println("result:", result)

	return Result(result), nil
}
