package cmd

import (
	"log"
	"plugin"
	"strings"

	"github.com/yomorun/yomo/internal/serverless"
)

type baseOptions struct {
	// Filename is the name of Serverless function file (default is app.go).
	Filename string
}

func buildServerlessFile(opts *baseOptions, args []string) (string, error) {
	if len(args) >= 1 && strings.HasSuffix(args[0], ".go") {
		// the second arg of `yomo build xxx.go` is a .go file
		opts.Filename = args[0]
	}

	// build the file first
	log.Print("Building the Serverless Function File...")
	soFile, err := serverless.Build(opts.Filename, true)
	if err != nil {
		log.Print("❌ Build the serverless file failure with err: ", err)
		return "", err
	}
	return soFile, nil
}

// buildAndLoadHandle builds the serverless file and load handler.
func buildAndLoadHandler(opts *baseOptions, args []string) (plugin.Symbol, error) {
	// build the file first
	soFile, err := buildServerlessFile(opts, args)
	if err != nil {
		return nil, err
	}

	// load handle
	slHandler, err := serverless.LoadHandler(soFile)
	if err != nil {
		log.Print("❌ Load handle from .so file failure with err: ", err)
		return nil, err
	}
	return slHandler, nil
}
