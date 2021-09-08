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

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/cli/pkg/log"
)

var meshConfURL string

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run a YoMo-Zipper",
	Long:  "Run a YoMo-Zipper",
	Run: func(cmd *cobra.Command, args []string) {
		if config == "" {
			log.FailureStatusEvent(os.Stdout, "Please input the file name of workflow config")
			return
		}
		conf, err := yomo.ParseConfig(config)
		if err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
			return
		}
		printYoMoServerConf(conf)

		// endpoint := fmt.Sprintf("%s:%d", conf.Host, conf.Port)

		log.InfoStatusEvent(os.Stdout, "Running YoMo-Zipper...")
		zipper := yomo.NewZipperServer(conf.Name)
		zipper.ConfigWorkflow(config)
		err = zipper.ListenAndServe()
		if err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
			return
		}
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)

	serveCmd.Flags().StringVarP(&config, "config", "c", "workflow.yaml", "Workflow config file")
	serveCmd.Flags().StringVarP(&meshConfURL, "mesh-config", "m", "", "The URL of mesh config")
	// serveCmd.MarkFlagRequired("config")
}

func printYoMoServerConf(wfConf *yomo.WorkflowConfig) {
	log.InfoStatusEvent(os.Stdout, "Found %d stream functions in YoMo-Zipper config", len(wfConf.Functions))
	for i, sfn := range wfConf.Functions {
		log.InfoStatusEvent(os.Stdout, "Stream Function %d: %s", i+1, sfn.Name)
	}
}
