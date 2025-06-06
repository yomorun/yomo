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
	Hidden:             true,
	FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
	Run: func(cmd *cobra.Command, args []string) {
		loadOptionsFromViper(viper.RunViper, &opts)
		if err := parseFileArg(&opts); err != nil {
			log.FailureStatusEvent(os.Stdout, "%s", err.Error())
			return
		}
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

		// if has `--production` flag, skip s.Build() process
		isProduction := opts.Production
		if !isProduction {
			// build
			log.PendingStatusEvent(os.Stdout, "Building YoMo Stream Function instance...")
			if err := s.Build(true); err != nil {
				log.FailureStatusEvent(os.Stdout, "%s", err.Error())
				os.Exit(127)
			}
			log.SuccessStatusEvent(os.Stdout, "YoMo Stream Function build successful!")
		} else {
			log.InfoStatusEvent(os.Stdout, "YoMo Serverless LLM Function is running in [production] mode")
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
	devCmd.Flags().StringVarP(&opts.Runtime, "runtime", "r", "node", "serverless runtime type")
	devCmd.Flags().BoolVarP(&opts.Production, "production", "p", false, "run in production mode, skip the build process")

	viper.BindPFlags(viper.DevViper, devCmd.Flags())
}
