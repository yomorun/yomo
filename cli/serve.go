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
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/yomorun/yomo"
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
		// printYoMoServerConf(conf)

		// endpoint := fmt.Sprintf("%s:%d", conf.Host, conf.Port)

		zipper, err := yomo.NewZipper(config)
		if err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
		}
		// auth
		auth := v.GetString("auth")
		if len(auth) > 0 {
			idx := strings.Index(auth, ":")
			if idx != -1 {
				authName := auth[:idx]
				idx++
				args := auth[idx:]
				authArgs := strings.Split(args, ",")
				// log.InfoStatusEvent(os.Stdout, "authName=%s, authArgs=%s, idx=%d", authName, authArgs, idx)
				zipper.InitOptions(yomo.WithAuth(authName, authArgs...))
			}
		}
		// mesh
		err = zipper.ConfigMesh(meshConfURL)
		if err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
		}

		log.InfoStatusEvent(os.Stdout, "Running YoMo-Zipper...")
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
	// auth string
	serveCmd.Flags().StringP("auth", "a", "", "authentication name and arguments, eg: `token:yomo`")
	v = viper.New()
	v.AutomaticEnv()
	v.SetEnvPrefix("YOMO")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.BindPFlag("auth", serveCmd.Flags().Lookup("auth"))
}
