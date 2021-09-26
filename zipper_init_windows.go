//go:build windows
// +build windows

package yomo

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/yomorun/yomo/pkg/logger"
)

// initialize when zipper running as server. support inspection:
// - `kill -SIGTERM <pid>` graceful shutdown
func (z *zipper) init() {
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
		logger.Printf("%sListening SIGUSR1, SIGUSR2, SIGTERM/SIGINT...", zipperLogPrefix)
		for p1 := range c {
			logger.Printf("Received signal: %s", p1)
			if p1 == syscall.SIGTERM || p1 == syscall.SIGINT {
				logger.Printf("graceful shutting down ... %s", p1)
				os.Exit(0)
			}
		}
	}()
}
