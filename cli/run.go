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
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/yomorun/yomo/pkg/log"

	// serverless registrations
	"github.com/yomorun/yomo/cli/serverless"
	_ "github.com/yomorun/yomo/cli/serverless/deno"
	_ "github.com/yomorun/yomo/cli/serverless/exec"
	_ "github.com/yomorun/yomo/cli/serverless/golang"
	_ "github.com/yomorun/yomo/cli/serverless/wasm"
	"github.com/yomorun/yomo/cli/viper"
)

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run [flags] sfn",
	Short: "Run a YoMo Stream Function",
	Long:  "Run a YoMo Stream Function",
	Run: func(cmd *cobra.Command, args []string) {
		if err := parseFileArg(args, &opts, defaultSFNCompliedFile, defaultSFNWASIFile, defaultSFNSourceFile); err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
			return
		}
		loadOptionsFromViper(viper.RunViper, &opts)
		// Serverless
		log.InfoStatusEvent(os.Stdout, "YoMo Stream Function file: %v", opts.Filename)
		if opts.Name == "" {
			log.FailureStatusEvent(os.Stdout, "YoMo Stream Function's Name is empty, please set name used by `-n` flag")
			return
		}
		// resolve serverless
		log.PendingStatusEvent(os.Stdout, "Creating YoMo Stream Function instance...")
		if err := parseZipperAddr(&opts); err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
			return
		}
		s, err := serverless.Create(&opts)
		if err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
			return
		}
		if !s.Executable() {
			log.FailureStatusEvent(os.Stdout,
				"You cannot run `%s` directly. build first with the `yomo build %s` command and then run with the 'yomo run %s' command.",
				opts.Filename,
				opts.Filename,
				opts.Filename,
			)
			return
		}
		// build if it's go file
		if ext := filepath.Ext(opts.Filename); ext == ".go" {
			log.PendingStatusEvent(os.Stdout, "Bingding YoMo Stream Function instance...")
			if err := s.Build(true); err != nil {
				log.FailureStatusEvent(os.Stdout, err.Error())
				os.Exit(127)
			}
			log.SuccessStatusEvent(os.Stdout, "YoMo Stream Function build successful!")
		}
		// run
		// wasi
		if ext := filepath.Ext(opts.Filename); ext == ".wasm" {
			wasmRuntime := opts.Runtime
			if wasmRuntime == "" {
				wasmRuntime = "wazero"
			}
			log.InfoStatusEvent(os.Stdout, "WASM runtime: %s", wasmRuntime)
		}
		log.InfoStatusEvent(
			os.Stdout,
			"Starting YoMo Stream Function instance, connecting to zipper: %v",
			opts.ZipperAddr,
		)
		log.InfoStatusEvent(os.Stdout, "Stream Function is running...")
		if err := s.Run(verbose); err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
			return
		}
	},
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().StringVarP(&opts.ZipperAddr, "zipper", "z", "localhost:9000", "YoMo-Zipper endpoint addr")
	runCmd.Flags().StringVarP(&opts.Name, "name", "n", "app", "yomo stream function name.")
	runCmd.Flags().StringVarP(&opts.ModFile, "modfile", "m", "", "custom go.mod")
	runCmd.Flags().StringVarP(&opts.Credential, "credential", "d", "", "client credential payload, eg: `token:dBbBiRE7`")
	runCmd.Flags().StringVarP(&opts.Runtime, "runtime", "r", "", "serverless runtime type")

	viper.BindPFlags(viper.RunViper, runCmd.Flags())
}
