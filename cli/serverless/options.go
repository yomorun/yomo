package serverless

// Options the for serverless command arguments.
type Options struct {
	Filename  string
	Host      string
	Port      int
	Name      string
	ModFile   string
	Arguments []string
}
