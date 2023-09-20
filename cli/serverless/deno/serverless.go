// Package deno provides a js/ts serverless runtime
package deno

import (
	"github.com/yomorun/yomo/cli/serverless"
)

// denoServerless will start deno program to run serverless functions.
type denoServerless struct {
	name       string
	fileName   string
	zipperAddr string
	credential string
}

// Init initializes the serverless
func (s *denoServerless) Init(opts *serverless.Options) error {
	s.name = opts.Name
	s.fileName = opts.Filename
	s.zipperAddr = opts.ZipperAddr
	s.credential = opts.Credential
	return nil
}

// Build is an empty implementation
func (s *denoServerless) Build(clean bool) error {
	return nil
}

// Run the wasm serverless function
func (s *denoServerless) Run(verbose bool) error {
	return run(s.name, s.zipperAddr, s.credential, s.fileName, "./"+s.name+".sock")
}

// Executable shows whether the program needs to be built
func (s *denoServerless) Executable() bool {
	return true
}

func init() {
	serverless.Register(&denoServerless{}, ".js", ".ts")
}
