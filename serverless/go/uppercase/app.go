package main

import (
	"errors"
	"fmt"
	"strings"
)

type Arguments string

type Result string

func Handler(args Arguments) (Result, error) {
	fmt.Println("args:", args)

	if len(args) > 20 {
		return "", errors.New("input too long")
	}

	result := strings.ToUpper(string(args))
	fmt.Println("result:", result)

	return Result(result), nil
}
