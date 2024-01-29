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
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/core/ai"
	"github.com/yomorun/yomo/core/router"
	pkgconfig "github.com/yomorun/yomo/pkg/config"
	"github.com/yomorun/yomo/pkg/log"
	"github.com/yomorun/yomo/pkg/trace"

	// TODO: need dynamic load
	_ "github.com/yomorun/yomo/pkg/ai/azopenai"
)

// TEST: need delete
type Msg struct {
	CityName string `json:"city_name" jsonschema:"description=The name of the city to be queried"`
}

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
		tp, shutdown, err := trace.NewTracerProvider("yomo-zipper")
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

		zipper, err := yomo.NewZipper(conf.Name, router.Default(), nil, conf.Mesh, options...)
		if err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
			return
		}
		zipper.Logger().Info("using config file", "file_path", config)

		// TODO: AI Server
		go func() {
			// "{\"name\":\"get-weather\",\"description\":\"Get the current weather for `city_name`\",\"parameters\":{\"type\":\"object\",\"properties\":{\"city_name\":{\"type\":\"string\",\"description\":\"The name of the city to be queried\"}},\"required\":[\"city_name\"]}}"
			// appID := "appID"
			// name := "get-weather"
			// tag := uint32(0x60)
			// description := "Get the current weather for `city_name`"
			// err := ai.RegisterFunctionCaller(appID, tag, name, description, &Msg{})
			// if err != nil {
			// 	log.FailureStatusEvent(os.Stdout, err.Error())
			// 	return
			// }
			err := ai.Serve()
			if err != nil {
				log.FailureStatusEvent(os.Stdout, err.Error())
				return
			}
			fmt.Println("AI Server is running...")
		}()

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
