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
	"github.com/yomorun/yomo/pkg/file"
	"github.com/yomorun/yomo/pkg/log"

	// serverless registrations
	_ "github.com/yomorun/yomo/cli/serverless/deno"
	_ "github.com/yomorun/yomo/cli/serverless/exec"
	_ "github.com/yomorun/yomo/cli/serverless/golang"
	_ "github.com/yomorun/yomo/cli/serverless/wasm"
)

var runViper *viper.Viper

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run a YoMo Stream Function",
	Long:  "Run a YoMo Stream Function",
	Run: func(cmd *cobra.Command, args []string) {
		loadViperValue(cmd, runViper, &opts.Filename, "file-name")
		loadViperValue(cmd, runViper, &url, "zipper")
		loadViperValue(cmd, runViper, &opts.Name, "name")
		loadViperValue(cmd, runViper, &opts.ModFile, "modfile")
		loadViperValue(cmd, runViper, &opts.Credential, "credential")
		loadViperValue(cmd, runViper, &opts.Runtime, "runtime")

		if opts.Name == "" {
			log.FailureStatusEvent(os.Stdout, "YoMo Stream Function name must be set.")
			return
		}

		if len(args) > 0 {
			opts.Filename = args[0]
		}
		// Serverless
		log.InfoStatusEvent(os.Stdout, "YoMo Stream Function file: %v", opts.Filename)
		if !file.IsExec(opts.Filename) && opts.Name == "" {
			log.FailureStatusEvent(os.Stdout, "YoMo Stream Function's Name is empty, please set name used by `-n` flag")
			return
		}
		// resolve serverless
		log.PendingStatusEvent(os.Stdout, "Create YoMo Stream Function instance...")
		if err := parseURL(url, &opts); err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
			return
		}
		s, err := serverless.Create(&opts)
		if err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
			return
		}
		if !s.Executable() {
			log.InfoStatusEvent(os.Stdout,
				"Starting YoMo Stream Function instance with Name: %s. Zipper: %v.",
				opts.Name,
				opts.ZipperAddrs,
			)
			// build
			log.PendingStatusEvent(os.Stdout, "YoMo Stream Function building...")
			if err := s.Build(true); err != nil {
				log.FailureStatusEvent(os.Stdout, err.Error())
				return
			}
			log.SuccessStatusEvent(os.Stdout, "Success! YoMo Stream Function build.")
		} else { // executable
			log.InfoStatusEvent(os.Stdout,
				"Starting YoMo Stream Function instance with executable file: %s. Zipper: %v.",
				opts.Filename,
				opts.ZipperAddrs,
			)
		}
		// run
		log.InfoStatusEvent(os.Stdout, "YoMo Stream Function is running...")
		if err := s.Run(verbose); err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
			return
		}
	},
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().StringVarP(&opts.Filename, "file-name", "f", "app.go", "Stream Function file")
	runCmd.Flags().StringVarP(&url, "zipper", "z", "localhost:9000", "YoMo-Zipper endpoint addr")
	runCmd.Flags().StringVarP(&opts.Name, "name", "n", "", "yomo stream function name. It should match the specific service name in YoMo-Zipper config (workflow.yaml)")
	runCmd.Flags().StringVarP(&opts.ModFile, "modfile", "m", "", "custom go.mod")
	runCmd.Flags().StringVarP(&opts.Credential, "credential", "d", "", "client credential payload, eg: `token:dBbBiRE7`")
	runCmd.Flags().StringVarP(&opts.Runtime, "runtime", "r", "", "serverless runtime type")

	runViper = bindViper(runCmd)
}
