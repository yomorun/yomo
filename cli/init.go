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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yomorun/yomo/cli/serverless"
	"github.com/yomorun/yomo/cli/template"
	"github.com/yomorun/yomo/cli/viper"
	"github.com/yomorun/yomo/pkg/file"
	"github.com/yomorun/yomo/pkg/log"
)

var sfnType string

// initCmd represents the init command
var initCmd = &cobra.Command{
	Use:   "init <app_name>",
	Short: "Initialize a YoMo Serverless LLM Function",
	Long:  "Initialize a YoMo Serverless LLM Function",
	Run: func(cmd *cobra.Command, args []string) {
		name := ""
		if len(args) >= 1 && args[0] != "" {
			name = args[0]
			opts.Name = name
		}
		if name == "" {
			log.FailureStatusEvent(os.Stdout, "Please input your app name, e.g. `yomo init my-tool [-r node -t llm]`")
			return
		}

		loadOptionsFromViper(viper.InitViper, &opts)

		log.PendingStatusEvent(os.Stdout, "Initializing the Serverless LLM Function with [%s] runtime...", opts.Runtime)
		name = strings.ReplaceAll(name, " ", "_")

		filename := filepath.Join(name, DefaultSFNSourceFile(opts.Runtime))
		opts.Filename = filename

		// create app source file
		fname := filepath.Join(name, DefaultSFNSourceFile(opts.Runtime))
		contentTmpl, err := template.GetContent("init", sfnType, opts.Runtime, false)
		if err != nil {
			log.FailureStatusEvent(os.Stdout, "%s", err.Error())
			return
		}
		// serverless setup
		err = serverless.Setup(&opts)
		if err != nil {
			log.FailureStatusEvent(os.Stdout, "%s", err.Error())
			return
		}
		if err := file.PutContents(fname, contentTmpl); err != nil {
			log.FailureStatusEvent(os.Stdout, "Write Serverless LLM Function into %s file failure with the error: %v", fname, err)
			return
		}
		// create app test file
		testName := filepath.Join(name, DefaultSFNTestSourceFile(opts.Runtime))
		testTmpl, err := template.GetContent("init", sfnType, opts.Runtime, true)
		if err != nil {
			if !errors.Is(err, template.ErrUnsupportedTest) {
				log.FailureStatusEvent(os.Stdout, "%s", err.Error())
				return
			}
		} else {
			if err := file.PutContents(testName, testTmpl); err != nil {
				log.FailureStatusEvent(os.Stdout, "Write unittest tmpl into %s file failure with the error: %v", testName, err)
				return
			}
		}
		// create .env
		fname = filepath.Join(name, ".env")
		var content []byte
		content = fmt.Appendf(content, "YOMO_SFN_RUNTIME=%s\n", opts.Runtime)
		content = fmt.Appendf(content, "YOMO_SFN_NAME=%s\n", name)
		content = fmt.Appendf(content, "YOMO_SFN_ZIPPER=%s\n", "localhost:9000")
		if err := file.PutContents(fname, content); err != nil {
			log.FailureStatusEvent(os.Stdout, "Write Serverless LLM Function .env file failure with the error: %v", err)
			return
		}

		log.SuccessStatusEvent(os.Stdout, "Congratulations! You have initialized the Serverless LLM Function successfully.")
		log.InfoStatusEvent(os.Stdout, "You can enjoy the YoMo Serverless LLM Function via the command: ")
		log.InfoStatusEvent(os.Stdout, "\tcd %s && yomo run", name)
	},
}

func init() {
	rootCmd.AddCommand(initCmd)

	initCmd.Flags().StringVarP(&sfnType, "type", "t", "llm", "The type of Serverless LLM Function, support normal and llm")
	initCmd.Flags().StringVarP(&opts.Runtime, "runtime", "r", "node", "serverless runtime type, support node and go")

	viper.BindPFlags(viper.InitViper, initCmd.Flags())
}
