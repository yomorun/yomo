// Package deno provides a js/ts serverless runtime
package deno

import (
	"os"
	"sync"

	"github.com/yomorun/yomo/cli/serverless"
	"github.com/yomorun/yomo/pkg/log"
)

// denoServerless will start deno program to run serverless functions.
type denoServerless struct {
	name        string
	fileName    string
	zipperAddrs []string
	credential  string
}

// Init initializes the serverless
func (s *denoServerless) Init(opts *serverless.Options) error {
	s.name = opts.Name
	s.fileName = opts.Filename
	s.zipperAddrs = opts.ZipperAddrs
	s.credential = opts.Credential
	return nil
}

// Build is an empty implementation
func (s *denoServerless) Build(clean bool) error {
	return nil
}

// Run the wasm serverless function
func (s *denoServerless) Run(verbose bool) error {
	var wg sync.WaitGroup

	for _, v := range s.zipperAddrs {
		wg.Add(1)
		go func(zipperAddr string) {
			err := run(s.name, zipperAddr, s.credential, s.fileName, "./"+s.name+".sock")
			if err != nil {
				log.FailureStatusEvent(os.Stderr, "%v", err)
			}
			wg.Done()
		}(v)
	}

	wg.Wait()
	return nil
}

// Executable shows whether the program needs to be built
func (s *denoServerless) Executable() bool {
	return true
}

func init() {
	serverless.Register(&denoServerless{}, ".js", ".ts")
}
