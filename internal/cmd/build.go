package cmd

import (
	"log"

	"github.com/spf13/cobra"
)

// BuildOptions are the options for build command.
type BuildOptions struct {
	baseOptions
}

// NewCmdBuild creates a new command build.
func NewCmdBuild() *cobra.Command {
	var opts = &BuildOptions{}

	var cmd = &cobra.Command{
		Use:   "build",
		Short: "Build the YoMo Serverless Function",
		Long:  "Build the YoMo Serverless Function as .so file",
		Run: func(cmd *cobra.Command, args []string) {
			_, err := buildServerlessFile(&opts.baseOptions, args)
			if err != nil {
				return
			}
			log.Print("✅ Build the serverless file successfully.")
		},
	}

	cmd.Flags().StringVar(&opts.Filename, "file-name", "app.go", "Serverless function file (default is app.go)")

	return cmd
}
