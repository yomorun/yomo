/*
Copyright © 2021 CELLA, Inc.

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
	Use:                "dev",
	Short:              "Dev a YoMo Stream Function",
	Long:               "Dev a YoMo Stream Function with mocking yomo-source data from YCloud.",
	FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
	Run: func(cmd *cobra.Command, args []string) {
		loadViperValue(cmd, devViper, &opts.Filename, "file-name")
		loadViperValue(cmd, devViper, &opts.ModFile, "modfile")

		if len(args) > 0 {
			opts.Filename = args[0]
		}
		// Serverless
		log.InfoStatusEvent(os.Stdout, "YoMo Stream Function file: %v", opts.Filename)
		// resolve serverless
		log.PendingStatusEvent(os.Stdout, "Create YoMo Stream Function instance...")

		// Connect the serverless to YoMo dev-server, it will automatically emit the mock data.
		opts.ZipperAddrs = []string{"dev.yomo.run:9140"}
		opts.Name = "yomo-app-demo"

		s, err := serverless.Create(&opts)
		if err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
			return
		}

		// build
		log.PendingStatusEvent(os.Stdout, "YoMo Stream Function building...")
		if err := s.Build(true); err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
			return
		}
		log.SuccessStatusEvent(os.Stdout, "Success! YoMo Stream Function build.")
		// run
		log.InfoStatusEvent(os.Stdout, "YoMo Stream Function is running...")
		if err := s.Run(verbose); err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
			return
		}
	},
}

func init() {
	rootCmd.AddCommand(devCmd)

	devCmd.Flags().StringVarP(&opts.Filename, "file-name", "f", "app.go", "Stream function file")
	devCmd.Flags().StringVarP(&opts.ModFile, "modfile", "m", "", "custom go.mod")

	devViper = bindViper(devCmd)
}
