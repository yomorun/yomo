//go:build !windows
// +build !windows

package yomo

import (
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/ylog"
	"github.com/yomorun/yomo/pkg/trace"
)

// initialize when zipper running as server. support inspection:
// - `kill -SIGUSR1 <pid>` inspect state()
// - `kill -SIGTERM <pid>` graceful shutdown
// - `kill -SIGUSR2 <pid>` inspect golang GC
func waitSignalForShutdownServer(server *core.Server) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGUSR2, syscall.SIGUSR1, syscall.SIGINT)
	ylog.Info("listening SIGUSR1, SIGUSR2, SIGTERM/SIGINT...")
	for p1 := range c {
		ylog.Debug("Received signal", "signal", p1)
		switch p1 {
		case syscall.SIGTERM, syscall.SIGINT:
			ylog.Debug("graceful shutting down ...", "sign", p1)
			// waiting for the server to finish processing the current request
			server.Close()
			trace.ShutdownTracerProvider()
			os.Exit(0)
		case syscall.SIGUSR2:
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			ylog.Debug("runtime stats", "gc_nums", m.NumGC)
		case syscall.SIGUSR1:
			statsToLogger(server)
		}
	}
}
