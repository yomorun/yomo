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
	"github.com/spf13/viper"
	"github.com/yomorun/yomo/cli/serverless"
	"github.com/yomorun/yomo/pkg/log"
)

var devViper *viper.Viper

// devCmd represents the dev command
var devCmd = &cobra.Command{
	Use:                "dev [flags] sfn.wasm",
	Short:              "Test a YoMo Stream Function",
	Long:               "Test a YoMo Stream Function with public zipper and mocking data",
	FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
	Run: func(cmd *cobra.Command, args []string) {
		if err := parseFileArg(args, &opts, defaultSFNCompliedFile); err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
			return
		}
		loadViperValue(cmd, devViper, &opts.ModFile, "modfile")
		// Serverless
		log.InfoStatusEvent(os.Stdout, "YoMo Stream Function file: %v", opts.Filename)
		// resolve serverless
		log.PendingStatusEvent(os.Stdout, "Create YoMo Stream Function instance...")

		// Connect the serverless to YoMo dev-server, it will automatically emit the mock data.
		opts.ZipperAddrs = []string{"tap.yomo.dev:9140"}
		opts.Name = "yomo-app-demo"

		s, err := serverless.Create(&opts)
		if err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
			return
		}
		if !s.Executable() {
			log.FailureStatusEvent(os.Stdout,
				"You cannot run `%s` directly. build first with the `yomo build %s` command and then run with the 'yomo run sfn.wasm' command.",
				opts.Filename,
				opts.Filename,
			)
			return
		}
		// run
		log.InfoStatusEvent(
			os.Stdout,
			"Starting YoMo Stream Function instance with executable file: %s. Zipper: %v.",
			opts.Filename,
			opts.ZipperAddrs,
		)
		log.InfoStatusEvent(os.Stdout, "YoMo Stream Function is running...")
		if err := s.Run(verbose); err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
			return
		}
	},
}

func init() {
	rootCmd.AddCommand(devCmd)

	devCmd.Flags().StringVarP(&opts.ModFile, "modfile", "m", "", "custom go.mod")

	devViper = bindViper(devCmd)
}
