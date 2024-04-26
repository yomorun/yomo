package serverless

// Options describles the command arguments of serverless.
type Options struct {
	// Filename is the path to the serverless file.
	Filename string
	// ZipperAddrs is the address of the zipper server
	ZipperAddr string
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
	// WASI build with WASI target
	WASI bool
}
