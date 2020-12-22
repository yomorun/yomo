package main

import (
	"github.com/spf13/cobra"
	"github.com/yomorun/yomo/internal/cmd"
)

func main() {
	rootCmd := &cobra.Command{Use: "yomo"}
	rootCmd.AddCommand(
		cmd.NewCmdBuild(),
		cmd.NewCmdDev(),
		cmd.NewCmdRun(),
	)
	rootCmd.Execute()
}
