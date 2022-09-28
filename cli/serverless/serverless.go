package serverless

import (
	"fmt"
	"path/filepath"
	"sync"
)

var (
	driversMu sync.RWMutex
	drivers   = make(map[string]Serverless)
)

// Serverless defines the interface for serverless
type Serverless interface {
	// Init initializes the serverless
	Init(opts *Options) error

	// Build compiles the serverless to executable
	Build(clean bool) error

	// Run compiles and runs the serverless
	Run(verbose bool) error

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
