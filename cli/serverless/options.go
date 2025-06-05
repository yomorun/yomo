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
	// Production indicates whether to run in production mode
	Production bool
}
