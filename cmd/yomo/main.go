package main

import (
	"github.com/spf13/cobra"
	"github.com/yomorun/yomo/internal/cmd"
	"github.com/yomorun/yomo/internal/cmd/wf"
)

func main() {
	rootCmd := &cobra.Command{
		Use:     "yomo",
		Version: cmd.GetVersion(),
	}

	// workflow
	wfCmd := &cobra.Command{
		Use:   "wf",
		Short: "Wf is the commands for YoMo workflow",
		Long:  "Wf is the commands for YoMo workflow.",
	}
	wfCmd.AddCommand(
		wf.NewCmdRun(),
		wf.NewCmdDev(),
	)

	// add commands to root
	rootCmd.AddCommand(
		cmd.NewCmdInit(),
		cmd.NewCmdBuild(),
		cmd.NewCmdDev(),
		cmd.NewCmdRun(),
		cmd.NewCmdVersion(),
		wfCmd,
	)
	rootCmd.Execute()
}
