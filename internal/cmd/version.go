package cmd

import (
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
)

// NewCmdVersion creates a new command version.
func NewCmdVersion() *cobra.Command {
	var cmd = &cobra.Command{
		Use:    "version",
		Hidden: true,
		Run: func(cmd *cobra.Command, args []string) {
			version := strings.TrimPrefix(Version, "v")
			if Date != "" {
				version = fmt.Sprintf("%s (%s)", version, Date)
			}
			fmt.Println("yomo version", version)
		},
	}

	return cmd
}

// Version is dynamically set by the toolchain or overridden by the Makefile.
var Version = "DEV"

// Date is dynamically set at build time in the Makefile.
var Date = "" // YYYY-MM-DD

func init() {
	if Version == "DEV" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "(devel)" {
			Version = "v" + info.Main.Version
		}
	}
}
