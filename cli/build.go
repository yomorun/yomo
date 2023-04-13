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

var buildViper *viper.Viper

// buildCmd represents the build command
var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build the YoMo Stream Function",
	Long:  "Build the YoMo Stream Function as binary file",
	Run: func(cmd *cobra.Command, args []string) {
		loadViperValue(cmd, buildViper, &opts.Filename, "file-name")
		// loadViperValue(cmd, buildViper, &url, "url")
		// loadViperValue(cmd, buildViper, &opts.Name, "name")
		loadViperValue(cmd, buildViper, &opts.ModFile, "modfile")
		// loadViperValue(cmd, buildViper, &opts.Credential, "credential")
		// use environment variable to override flags
		opts.UseEnv = true

		// if opts.Name == "" {
		// 	log.FailureStatusEvent(os.Stdout, "YoMo Stream Function name must be set.")
		// 	return
		// }

		if len(args) > 0 {
			opts.Filename = args[0]
		}
		log.InfoStatusEvent(os.Stdout, "YoMo Stream Function file: %v", opts.Filename)
		// resolve serverless
		// log.PendingStatusEvent(os.Stdout, "Create YoMo Stream Function instance...")
		// if err := parseURL(url, &opts); err != nil {
		// 	log.FailureStatusEvent(os.Stdout, err.Error())
		// 	return
		// }
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
	},
}

func init() {
	rootCmd.AddCommand(buildCmd)

	buildCmd.Flags().StringVarP(&opts.Filename, "file-name", "f", "app.go", "Stream function file (default is app.go)")
	// buildCmd.Flags().StringVarP(&url, "url", "u", "localhost:9000", "YoMo-Zipper endpoint addr")
	// buildCmd.Flags().StringVarP(&opts.Name, "name", "n", "", "yomo stream function app name (required). It should match the specific service name in YoMo-Zipper config (workflow.yaml)")
	buildCmd.Flags().StringVarP(&opts.ModFile, "modfile", "m", "", "custom go.mod")
	// buildCmd.Flags().StringVarP(&opts.Credential, "credential", "d", "", "client credential payload, eg: `token:dBbBiRE7`")
	buildCmd.Flags().StringVarP(&opts.Target, "target", "t", "", "Build target: native or wasm")

	buildViper = bindViper(buildCmd)
}
