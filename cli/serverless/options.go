package serverless

// Options describles the command arguments of serverless.
type Options struct {
	// Filename is the path to the serverless.yml file.
	Filename string
	// Host is the hostname to listen on or connect to.
	Host string
	// Port is the port to listen on or connect to.
	Port int
	// Name is the name of the service.
	Name string
	// ModFile is the path to the module file.
	ModFile string
	// Arguments are the command line arguments.
	Arguments []string
}
