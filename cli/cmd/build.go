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
package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/yomorun/yomo/cli/pkg/log"
	"github.com/yomorun/yomo/cli/serverless"
)

// buildCmd represents the build command
var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build the YoMo Stream Function",
	Long:  "Build the YoMo Stream Function as binary file",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) > 0 {
			opts.Filename = args[0]
		}
		log.InfoStatusEvent(os.Stdout, "YoMo Stream Function file: %v", opts.Filename)
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
		log.InfoStatusEvent(os.Stdout,
			"Starting YoMo Stream Function instance with Name: %s. Host: %s. Port: %d.",
			opts.Name,
			opts.Host,
			opts.Port,
		)
		// build
		log.PendingStatusEvent(os.Stdout, "YoMo Stream Function function building...")
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
	buildCmd.Flags().StringVarP(&url, "url", "u", "localhost:9000", "YoMo-Zipper endpoint addr")
	buildCmd.Flags().StringVarP(&opts.Name, "name", "n", "", "yomo stream function app name (required). It should match the specific service name in YoMo-Zipper config (workflow.yaml)")
	buildCmd.MarkFlagRequired("name")
	buildCmd.Flags().StringVarP(&opts.ModFile, "modfile", "m", "", "custom go.mod")
}
