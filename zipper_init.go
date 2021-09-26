//go:build !windows
// +build !windows

package yomo

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/yomorun/yomo/pkg/logger"
)

// initialize when zipper running as server. support inspection:
// - `kill -SIGUSR1 <pid>` inspect state()
// - `kill -SIGTERM <pid>` graceful shutdown
// - `kill -SIGUSR2 <pid>` inspect golang GC
func (z *zipper) init() {
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGUSR2, syscall.SIGUSR1, syscall.SIGINT)
		logger.Printf("%sListening SIGUSR1, SIGUSR2, SIGTERM/SIGINT...", zipperLogPrefix)
		for p1 := range c {
			logger.Printf("Received signal: %s", p1)
			if p1 == syscall.SIGTERM || p1 == syscall.SIGINT {
				logger.Printf("graceful shutting down ... %s", p1)
				os.Exit(0)
				// close(sgnl)
			} else if p1 == syscall.SIGUSR2 {
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				fmt.Printf("\tNumGC = %v\n", m.NumGC)
			} else if p1 == syscall.SIGUSR1 {
				logger.Printf("print zipper stats(): %d", z.Stats())
			}
		}
	}()
}
