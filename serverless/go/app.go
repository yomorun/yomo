package main

const Description = "Hello, YoMo!"

type Arguments struct{}

type Result struct{}

func Handler(args Arguments) (Result, error) {
	return Result{}, nil
}
