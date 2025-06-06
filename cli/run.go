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
	"github.com/yomorun/yomo/pkg/log"

	// serverless registrations
	"github.com/yomorun/yomo/cli/serverless"
	_ "github.com/yomorun/yomo/cli/serverless/golang"
	_ "github.com/yomorun/yomo/cli/serverless/nodejs"
	"github.com/yomorun/yomo/cli/viper"
)

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run [flags] sfn",
	Short: "Run a YoMo Serverless LLM Function",
	Long:  "Run a YoMo Serverless LLM Function",
	Run: func(cmd *cobra.Command, args []string) {
		loadOptionsFromViper(viper.RunViper, &opts)
		if err := parseFileArg(&opts); err != nil {
			log.FailureStatusEvent(os.Stdout, "%s", err.Error())
			return
		}
		// Serverless
		log.InfoStatusEvent(os.Stdout, "YoMo Serverless LLM Function file: %v", opts.Filename)
		if opts.Name == "" {
			log.FailureStatusEvent(os.Stdout, "YoMo Serverless LLM Function's Name is empty, please set name used by `-n` flag")
			return
		}
		// resolve serverless
		log.PendingStatusEvent(os.Stdout, "Creating YoMo Serverless LLM Function instance...")
		if err := parseZipperAddr(&opts); err != nil {
			log.FailureStatusEvent(os.Stdout, "%s", err.Error())
			return
		}
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

		log.InfoStatusEvent(
			os.Stdout,
			"Starting YoMo Serverless LLM Function instance, connecting to zipper: %v",
			opts.ZipperAddr,
		)
		log.InfoStatusEvent(os.Stdout, "Serverless LLM Function is running...")
		if err := s.Run(verbose); err != nil {
			log.FailureStatusEvent(os.Stdout, "%s", err.Error())
			return
		}
	},
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().StringVarP(&opts.ZipperAddr, "zipper", "z", "localhost:9000", "YoMo-Zipper endpoint addr")
	runCmd.Flags().StringVarP(&opts.Name, "name", "n", "", "yomo Serverless LLM Function name.")
	runCmd.Flags().StringVarP(&opts.ModFile, "modfile", "m", "", "custom go.mod")
	runCmd.Flags().StringVarP(&opts.Credential, "credential", "d", "", "client credential payload, eg: `token:dBbBiRE7`")
	runCmd.Flags().StringVarP(&opts.Runtime, "runtime", "r", "node", "serverless runtime type")
	runCmd.Flags().BoolVarP(&opts.Production, "production", "p", false, "run in production mode, skip the build process")

	viper.BindPFlags(viper.RunViper, runCmd.Flags())
}
