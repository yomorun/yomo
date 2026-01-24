package main

type Arguments struct{}

type Result struct{}

var ServerlessContext map[string]any

func Handler(args Arguments) (Result, error) {
	return Result{}, nil
}

const ServerlessMode = "simple"
