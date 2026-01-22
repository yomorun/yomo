package main

import (
	"errors"
	"log"
	"strings"
	"time"
)

func SimpleHandler(args string) (string, error) {
	log.Println("[Go SimpleHandler] args:", args)

	if len(args) > 20 {
		return "", errors.New("input too long")
	}

	result := strings.ToUpper(args)
	log.Println("[Go SimpleHandler] result:", result)

	return result, nil
}

func StreamHandler(args string, ch chan<- string) error {
	log.Println("[Go StreamHandler] args:", args)

	for _, chunk := range strings.Split(args, " ") {
		if len(chunk) > 20 {
			return errors.New("chunk too long")
		}

		chunkResult := strings.ToUpper(chunk)
		log.Println("[Go StreamHandler] chunk result:", chunkResult)

		ch <- chunkResult
		time.Sleep(time.Second)
	}

	return nil
}
