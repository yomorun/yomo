package main

import (
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
			return fmt.Errorf("chunk '%s' is too long", chunk)
		}

		chunkResult := strings.ToUpper(chunk)
		fmt.Println("chunk result:", chunkResult)

		ch <- Result(chunkResult)
		time.Sleep(time.Second)
	}

	return nil
}
