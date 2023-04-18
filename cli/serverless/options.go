package serverless

// Options describles the command arguments of serverless.
type Options struct {
	// Filename is the path to the serverless file.
	Filename string
	// ZipperAddrs is the addresses of the zipper server
	ZipperAddrs []string
	// Name is the name of the service.
	Name string
	// ModFile is the path to the module file.
	ModFile string
	// Client credential
	Credential string
	// Runtime specifies the serverless runtime environment type
	Runtime string
	// use environment variables
	UseEnv bool
	// Builder build wasm by: gojs or tinygo
	Builder string
}
