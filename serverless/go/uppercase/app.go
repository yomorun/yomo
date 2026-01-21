package main

import (
	"log"
	"strings"
	"time"
)

func SimpleHandler(args string) (string, error) {
	log.Println("[SimpleHandler] args:", args)

	result := strings.ToUpper(args)
	log.Println("[SimpleHandler] result:", result)

	return result, nil
}

func StreamHandler(args string, ch chan<- string) error {
	log.Println("[StreamHandler] args:", args)

	for _, x := range strings.Split(args, " ") {
		chunkResult := strings.ToUpper(x)
		log.Println("[StreamHandler] chunk result:", chunkResult)

		ch <- chunkResult
		time.Sleep(time.Second)
	}

	return nil
}
