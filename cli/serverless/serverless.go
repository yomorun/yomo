package serverless

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/joho/godotenv"
	"github.com/yomorun/yomo/pkg/file"
)

var (
	driversMu sync.RWMutex
	drivers   = make(map[string]Serverless)
)

// Serverless defines the interface for serverless
type Serverless interface {
	// Setup sets up the serverless
	Setup(opts *Options) error

	// Init initializes the serverless
	Init(opts *Options) error

	// Build compiles the serverless to executable
	Build(clean bool) error

	// Run compiles and runs the serverless
	Run(verbose bool) error

	// Executable returns true if the serverless is executable
	Executable() bool
}

// Register will register a serverless to drivers collections safely
func Register(s Serverless, exts ...string) {
	driversMu.Lock()
	defer driversMu.Unlock()
	if s == nil {
		panic("serverless: Register serverless is nil")
	}
	for _, ext := range exts {
		if _, dup := drivers[ext]; dup {
			panic("serverless: Register called twice for source " + ext)
		}
		drivers[ext] = s
	}
}

// LoadEnvFile loads the environment variables from the file
func LoadEnvFile(envDir string) error {
	envFile := filepath.Join(envDir, ".env")
	if file.Exists(envFile) {
		return godotenv.Load(envFile)
	}
	return nil
}

// Create returns a new serverless instance with options.
func Create(opts *Options) (Serverless, error) {
	ext := filepath.Ext(opts.Filename)

	driversMu.RLock()
	s, ok := drivers[ext]
	driversMu.RUnlock()
	if ok {
		if err := s.Init(opts); err != nil {
			return nil, err
		}
		return s, nil
	}

	return nil, fmt.Errorf(`serverless: unsupport "%s" source (forgotten import?)`, ext)
}

// Setup sets up the serverless
func Setup(opts *Options) error {
	ext := filepath.Ext(opts.Filename)

	driversMu.RLock()
	s, ok := drivers[ext]
	driversMu.RUnlock()
	if ok {
		return s.Setup(opts)
	}

	return fmt.Errorf(`serverless: unsupport "%s" source (forgotten import?)`, ext)
}
