// Package wasm provides WebAssembly serverless function runtimes.
package wasm

import (
	"log"
	"os"
	"sync"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/cli/serverless"
	pkglog "github.com/yomorun/yomo/pkg/log"
)

// wasmServerless will run serverless functions from the given compiled WebAssembly files.
type wasmServerless struct {
	runtime     Runtime
	name        string
	zipperAddrs []string
	observed    []uint32
	credential  string
}

// Init initializes the serverless
func (s *wasmServerless) Init(opts *serverless.Options) error {
	runtime, err := NewRuntime(opts.Runtime)
	if err != nil {
		return err
	}

	err = runtime.Init(opts.Filename)
	if err != nil {
		return err
	}

	s.runtime = runtime
	s.name = opts.Name
	s.zipperAddrs = opts.ZipperAddrs
	s.observed = runtime.GetObserveDataTags()
	s.credential = opts.Credential

	return nil
}

// Build is an empty implementation
func (s *wasmServerless) Build(clean bool) error {
	return nil
}

func wasmHandler(runtime Runtime, errCh chan error) func(req []byte) (uint32, []byte) {
	return func(req []byte) (uint32, []byte) {
		instance, err := runtime.Instance()
		if err != nil {
			log.Printf("[wasm] instance error: %v\n", err)
			errCh <- err
			return 0, nil
		}
		defer instance.Close()
		tag, res, err := instance.RunHandler(req)
		if err != nil {
			log.Printf("[wasm] run handler error: %v\n", err)
			errCh <- err
		}
		return tag, res
	}
}

// Run the wasm serverless function
func (s *wasmServerless) Run(verbose bool) error {
	var wg sync.WaitGroup

	for _, addr := range s.zipperAddrs {
		sfn := yomo.NewStreamFunction(
			s.name,
			addr,
			yomo.WithSfnCredential(s.credential),
		)
		sfn.SetObserveDataTags(s.observed...)

		var ch chan error
		// run wasm handler
		sfn.SetHandler(wasmHandler(s.runtime, ch))

		sfn.SetErrorHandler(
			func(err error) {
				log.Printf("[wasm][%s] error handler: %T %v\n", addr, err, err)
			},
		)

		err := sfn.Connect()
		if err != nil {
			return err
		}
		defer sfn.Close()
		defer s.runtime.Close()

		wg.Add(1)
		go func() {
			err := <-ch
			if err != nil {
				pkglog.FailureStatusEvent(os.Stderr, "%v", err)
			}
			wg.Done()
		}()
	}

	wg.Wait()
	return nil
}

// Executable shows whether the program needs to be built
func (s *wasmServerless) Executable() bool {
	return true
}

func init() {
	serverless.Register(&wasmServerless{}, ".wasm")
}
