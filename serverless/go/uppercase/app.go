package main

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// The simple handler function that takes a string argument and returns a string result.
func SimpleHandler(args string, context string) (string, error) {
	fmt.Println("args:", args)

	if len(args) > 20 {
		return "", errors.New("input too long")
	}

	result := strings.ToUpper(args)
	fmt.Println("result:", result)

	return result, nil
}

// The stream handler function that takes a string argument and returns a stream of string results.
func StreamHandler(args string, context string, ch chan<- string) error {
	fmt.Println("args:", args)

	for _, chunk := range strings.Split(args, " ") {
		if len(chunk) > 20 {
			return errors.New("chunk too long")
		}

		chunkResult := strings.ToUpper(chunk)
		fmt.Println("chunk result:", chunkResult)

		ch <- chunkResult
		time.Sleep(time.Second)
	}

	return nil
}
