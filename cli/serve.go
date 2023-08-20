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

	"github.com/spf13/cobra"
	"github.com/yomorun/yomo"
	pkgconfig "github.com/yomorun/yomo/pkg/config"
	"github.com/yomorun/yomo/pkg/log"
	"github.com/yomorun/yomo/pkg/trace"
)

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run a YoMo-Zipper",
	Long:  "Run a YoMo-Zipper",
	Run: func(cmd *cobra.Command, args []string) {
		if config == "" {
			log.FailureStatusEvent(os.Stdout, "Please input the file name of config")
			return
		}

		log.InfoStatusEvent(os.Stdout, "Running YoMo-Zipper...")
		// config
		conf, err := pkgconfig.ParseConfigFile(config)
		if err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
			return
		}
		ctx := context.Background()
		// trace
		tp, shutdown, err := trace.NewTracerProviderWithJaeger("yomo-zipper")
		if err == nil {
			log.InfoStatusEvent(os.Stdout, "[zipper] 🛰 trace enabled")
		}
		defer shutdown(ctx)
		// listening address.
		listenAddr := fmt.Sprintf("%s:%d", conf.Host, conf.Port)

		options := []yomo.ZipperOption{yomo.WithZipperTracerProvider(tp)}
		if _, ok := conf.Auth["type"]; ok {
			if tokenString, ok := conf.Auth["token"]; ok {
				options = append(options, yomo.WithAuth("token", tokenString))
			}
		}

		zipper, err := yomo.NewZipper(conf.Name, conf.Functions, conf.Downstreams, options...)
		if err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
			return
		}
		zipper.Logger().Info("using config file", "file_path", config)

		err = zipper.ListenAndServe(ctx, listenAddr)
		if err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
			return
		}
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)

	serveCmd.Flags().StringVarP(&config, "config", "c", "", "config file")
}
