//go:build windows
// +build windows

package yomo

import (
	"os"
	"os/signal"
	"syscall"
)

// initialize when zipper running as server. support inspection:
// - `kill -SIGTERM <pid>` graceful shutdown
func (z *zipper) init() {
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGTERM, syscall.SIGINT)
		logger.Printf("%sListening SIGTERM/SIGINT...", zipperLogPrefix)
		for p1 := range c {
			fmt.Printf("Received signal: %s", p1)
			if p1 == syscall.SIGTERM || p1 == syscall.SIGINT {
				fmt.Printf("graceful shutting down ... %s", p1)
				os.Exit(0)
			}
		}
	}()
}
