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
	"github.com/yomorun/yomo/pkg/file"
	"github.com/yomorun/yomo/pkg/log"
)

var (
	sfnType string
	lang    string
)

// initCmd represents the init command
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a YoMo Stream function",
	Long:  "Initialize a YoMo Stream function",
	Run: func(cmd *cobra.Command, args []string) {
		name := opts.Name
		if len(args) >= 1 && args[0] != "" {
			name = args[0]
		}

		if name == "" {
			log.FailureStatusEvent(os.Stdout, "Please input your app name")
			return
		}

		log.PendingStatusEvent(os.Stdout, "Initializing the Stream Function...")
		name = strings.ReplaceAll(name, " ", "_")
		filename := filepath.Join(name, DefaultSFNSourceFile(lang))
		opts.Filename = filename
		// serverless setup
		err := serverless.Setup(&opts)
		if err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
			return
		}
		// create app source file
		fname := filepath.Join(name, DefaultSFNSourceFile(lang))
		contentTmpl, err := template.GetContent("init", sfnType, lang, false)
		if err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
			return
		}
		if err := file.PutContents(fname, contentTmpl); err != nil {
			log.FailureStatusEvent(os.Stdout, "Write stream function into %s file failure with the error: %v", fname, err)
			return
		}
		// create app test file
		testName := filepath.Join(name, DefaultSFNTestSourceFile(lang))
		testTmpl, err := template.GetContent("init", sfnType, lang, true)
		if err != nil {
			if !errors.Is(err, template.ErrUnsupportedTest) {
				log.FailureStatusEvent(os.Stdout, err.Error())
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
		if err := file.PutContents(fname, []byte(fmt.Sprintf("YOMO_SFN_NAME=%s\nYOMO_SFN_ZIPPER=localhost:9000\n", name))); err != nil {
			log.FailureStatusEvent(os.Stdout, "Write stream function .env file failure with the error: %v", err)
			return
		}

		log.SuccessStatusEvent(os.Stdout, "Congratulations! You have initialized the stream function successfully.")
		log.InfoStatusEvent(os.Stdout, "You can enjoy the YoMo Stream Function via the command: ")
		log.InfoStatusEvent(os.Stdout, "\tStep 1: cd %s && yomo build", name)
		log.InfoStatusEvent(os.Stdout, "\tStep 2: yomo run")
	},
}

func init() {
	rootCmd.AddCommand(initCmd)

	initCmd.Flags().StringVarP(&opts.Name, "name", "n", "", "The name of Stream Function")
	initCmd.Flags().StringVarP(&sfnType, "type", "t", "llm", "The type of Stream Function, support normal and llm")
	initCmd.Flags().StringVarP(&lang, "lang", "l", "go", "The language of Stream Function, support go and node")
}
