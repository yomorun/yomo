package nodejs

import (
	"github.com/yomorun/yomo/cli/serverless"
	"github.com/yomorun/yomo/pkg/wrapper"
)

// nodejsServerless will start js program to run serverless functions.
type nodejsServerless struct {
	name       string
	zipperAddr string
	credential string
	wrapper    *NodejsWrapper
}

// Init initializes the serverless
func (s *nodejsServerless) Init(opts *serverless.Options) error {
	wrapper, err := NewWrapper(opts.Name, opts.Filename)
	if err != nil {
		return err
	}

	s.name = opts.Name
	s.zipperAddr = opts.ZipperAddr
	s.credential = opts.Credential
	s.wrapper = wrapper

	return nil
}

// Build is an empty implementation
func (s *nodejsServerless) Build(_ bool) error {
	return s.wrapper.Build()
}

// Run the wasm serverless function
func (s *nodejsServerless) Run(verbose bool) error {
	return wrapper.Run(s.name, s.zipperAddr, s.credential, s.wrapper)
}

// Executable shows whether the program needs to be built
func (s *nodejsServerless) Executable() bool {
	return true
}

func init() {
	serverless.Register(&nodejsServerless{}, ".ts")
}
