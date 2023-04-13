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
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/yomorun/yomo"
	pkgconfig "github.com/yomorun/yomo/pkg/config"
	"github.com/yomorun/yomo/pkg/log"
)

var meshConfURL string
var v *viper.Viper

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

		// parse workflow config.
		wfg, err := pkgconfig.ParseWorkflowConfig(config)
		if err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
			return
		}

		// auth
		var serverOption []yomo.DownstreamZipperOption
		auth := v.GetString("auth")
		if len(auth) > 0 {
			idx := strings.Index(auth, ":")
			if idx != -1 {
				authName := auth[:idx]
				idx++
				args := auth[idx:]
				authArgs := strings.Split(args, ",")
				serverOption = append(serverOption, yomo.WithAuth(authName, authArgs...))
			}
		}

		zipper, err := yomo.NewZipper(wfg.Name, wfg.Functions, yomo.WithDownstreamOption(serverOption...))
		if err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
			return
		}

		log.InfoStatusEvent(os.Stdout, "Running YoMo-Zipper...")
		err = zipper.ListenAndServe(context.Background(), fmt.Sprintf("%s:%d", wfg.Host, wfg.Port))
		if err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
			return
		}
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)

	serveCmd.Flags().StringVarP(&config, "config", "c", "", "Workflow config file")
	serveCmd.Flags().StringVarP(&meshConfURL, "mesh-config", "m", "", "The URL of mesh config")
	// auth string
	serveCmd.Flags().StringP("auth", "a", "", "authentication name and arguments, eg: `token:yomo`")
	v = viper.New()
	v.AutomaticEnv()
	v.SetEnvPrefix("YOMO")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.BindPFlag("auth", serveCmd.Flags().Lookup("auth"))
}
