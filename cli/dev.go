/*
Copyright © 2021 Allegro Networks

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cli

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/yomorun/yomo/cli/serverless"
	"github.com/yomorun/yomo/cli/viper"
	"github.com/yomorun/yomo/pkg/log"
)

// devCmd represents the dev command
var devCmd = &cobra.Command{
	Use:                "dev [flags]",
	Short:              "Test a YoMo Stream Function",
	Long:               "Test a YoMo Stream Function with public zipper and mocking data",
	FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
	Run: func(cmd *cobra.Command, args []string) {
		if err := parseFileArg(args, &opts, defaultSFNCompliedFile, defaultSFNWASIFile, defaultSFNSourceFile); err != nil {
			log.FailureStatusEvent(os.Stdout, "%s", err.Error())
			return
		}
		loadOptionsFromViper(viper.RunViper, &opts)
		// Serverless
		log.InfoStatusEvent(os.Stdout, "YoMo Stream Function file: %v", opts.Filename)
		// resolve serverless
		log.PendingStatusEvent(os.Stdout, "Creating YoMo Stream Function instance...")

		// Connect the serverless to YoMo dev-server, it will automatically emit the mock data.
		opts.Name = "yomo-app-demo"
		opts.ZipperAddr = "tap.yomo.dev:9140"

		// Set the environment variables for the YoMo Stream Function
		os.Setenv("YOMO_SFN_NAME", opts.Name)
		os.Setenv("YOMO_SFN_ZIPPER", opts.ZipperAddr)

		s, err := serverless.Create(&opts)
		if err != nil {
			log.FailureStatusEvent(os.Stdout, "%s", err.Error())
			return
		}
		if !s.Executable() {
			log.FailureStatusEvent(os.Stdout,
				"You cannot run `%s` directly. build first with the `yomo build %s` command and then run with the 'yomo run %s' command.",
				opts.Filename,
				opts.Filename,
				opts.Filename,
			)
			return
		}
		// build if it's go file
		if ext := filepath.Ext(opts.Filename); ext == ".go" {
			log.PendingStatusEvent(os.Stdout, "Building YoMo Stream Function instance...")
			if err := s.Build(true); err != nil {
				log.FailureStatusEvent(os.Stdout, "%s", err.Error())
				os.Exit(127)
			}
			log.SuccessStatusEvent(os.Stdout, "YoMo Stream Function build successful!")
		}
		// run
		log.InfoStatusEvent(
			os.Stdout,
			"Starting YoMo Stream Function instance, connecting to zipper: %v",
			opts.ZipperAddr,
		)
		log.InfoStatusEvent(os.Stdout, "Stream Function is running...")
		if err := s.Run(verbose); err != nil {
			log.FailureStatusEvent(os.Stdout, "%s", err.Error())
			return
		}
	},
}

func init() {
	rootCmd.AddCommand(devCmd)

	devCmd.Flags().StringVarP(&opts.ModFile, "modfile", "m", "", "custom go.mod")

	viper.BindPFlags(viper.DevViper, devCmd.Flags())
}
