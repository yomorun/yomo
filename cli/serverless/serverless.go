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

type Serverless interface {
	Init(opts *Options) error
	Build(clean bool) error
	Run(verbose bool) error
}

func Register(ext string, s Serverless) {
	driversMu.Lock()
	defer driversMu.Unlock()
	if s == nil {
		panic("serverless: Register serverless is nil")
	}
	if _, dup := drivers[ext]; dup {
		panic("serverless: Register called twice for source " + ext)
	}
	drivers[ext] = s
}

func Create(opts *Options) (Serverless, error) {
	// isSource := false
	ext := filepath.Ext(opts.Filename)
	// if ext != "" && ext != ".exe" {
	// 	isSource = true
	// }

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
