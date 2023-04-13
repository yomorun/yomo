//go:build windows
// +build windows

package yomo

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/yomorun/yomo/core/ylog"
)

// initialize when zipper running as server. support inspection:
// - `kill -SIGTERM <pid>` graceful shutdown
func waitSignalForShotdownServer(server *core.Server) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGINT)
	ylog.Info("Listening SIGTERM/SIGINT...")
	for p1 := range c {
		ylog.Debug("Received signal", "signal", p1)
		if p1 == syscall.SIGTERM || p1 == syscall.SIGINT {
			// server.Close()
			ylog.Debug("graceful shutting down ...", "sign", p1)
			os.Exit(0)
		}
	}
}
