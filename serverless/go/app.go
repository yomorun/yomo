package main

type Arguments struct{}

type Result struct{}

func Handler(args Arguments) (Result, error) {
	return Result{}, nil
}
