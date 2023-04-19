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
	Use:   "build [flags] app.go",
	Short: "Build the YoMo Stream Function",
	Long:  "Build the YoMo Stream Function as WebAssembly",
	Run: func(cmd *cobra.Command, args []string) {
		if err := parseFileArg(args, &opts, defaultSFNSourceFile); err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
			return
		}
		loadViperValue(cmd, buildViper, &opts.ModFile, "modfile")
		// use environment variable to override flags
		opts.UseEnv = true

		log.InfoStatusEvent(os.Stdout, "YoMo Stream Function file: %v", opts.Filename)
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

	buildCmd.Flags().StringVarP(&opts.ModFile, "modfile", "m", "", "custom go.mod")
	buildCmd.Flags().StringVarP(&opts.Builder, "builder", "b", "tinygo", "Builder: use native gojs or tinygo")

	buildViper = bindViper(buildCmd)
}
