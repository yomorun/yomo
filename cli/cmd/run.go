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
	_ "github.com/yomorun/yomo/cli/serverless/golang"
)

const (
	runtimeWaitTimeoutInSeconds = 60
)

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run a YoMo Stream Function",
	Long:  "Run a YoMo Stream Function",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) > 0 {
			opts.Filename = args[0]
		}
		// os signal
		// sigCh := make(chan os.Signal, 1)
		// signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
		// Serverless
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
		// Exit
		// <-sigCh
		// log.WarningStatusEvent(os.Stdout, "Terminated signal received: shutting down")
		// log.InfoStatusEvent(os.Stdout, "Exited YoMo Stream Function instance.")
	},
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().StringVarP(&opts.Filename, "file-name", "f", "app.go", "Stream function file")
	// runCmd.Flags().StringVarP(&opts.Lang, "lang", "l", "go", "source language")
	runCmd.Flags().StringVarP(&url, "url", "u", "localhost:9000", "YoMo-Zipper endpoint addr")
	runCmd.Flags().StringVarP(&opts.Name, "name", "n", "", "yomo stream function name (required). It should match the specific service name in YoMo-Zipper config (workflow.yaml)")
	runCmd.Flags().StringVarP(&opts.ModFile, "modfile", "m", "", "custom go.mod")
	runCmd.MarkFlagRequired("name")

}
