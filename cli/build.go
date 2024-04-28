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

// buildCmd represents the build command
var buildCmd = &cobra.Command{
	Use:   "build [flags] app.go",
	Short: "Build the YoMo Stream Function",
	Long:  "Build the YoMo Stream Function",
	Run: func(cmd *cobra.Command, args []string) {
		if err := parseFileArg(args, &opts, defaultSFNSourceFile); err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
			os.Exit(127)
			// return
		}
		loadOptionsFromViper(viper.BuildViper, &opts)

		log.InfoStatusEvent(os.Stdout, "YoMo Stream Function file: %v", opts.Filename)
		s, err := serverless.Create(&opts)
		if err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
			os.Exit(127)
			// return
		}
		// build
		log.PendingStatusEvent(os.Stdout, "YoMo Stream Function building...")
		if err := s.Build(true); err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
			os.Exit(127)
			// return
		}
		log.SuccessStatusEvent(os.Stdout, "Success! YoMo Stream Function build.")
	},
}

func init() {
	rootCmd.AddCommand(buildCmd)

	buildCmd.Flags().StringVarP(&opts.ModFile, "modfile", "m", "", "custom go.mod")
	buildCmd.Flags().BoolVarP(&opts.WASI, "wasi", "w", false, "build with WASI target")

	viper.BindPFlags(viper.BuildViper, buildCmd.Flags())
}
