package main

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type Arguments string

type Result string

func Handler(args Arguments, ch chan<- Result) error {
	fmt.Println("Processing stream")

	for _, chunk := range strings.Split(string(args), " ") {
		if len(chunk) > 20 {
			return errors.New("chunk too long")
		}

		chunkResult := strings.ToUpper(chunk)
		fmt.Println("chunk result:", chunkResult)

		ch <- Result(chunkResult)
		time.Sleep(time.Second)
	}

	return nil
}
