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
	"github.com/yomorun/yomo/core/router"
	"github.com/yomorun/yomo/core/ylog"
	pkgconfig "github.com/yomorun/yomo/pkg/config"
	"github.com/yomorun/yomo/pkg/log"

	"github.com/yomorun/yomo/pkg/bridge/ai"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider/azopenai"
	// "github.com/yomorun/yomo/pkg/bridge/ai/provider/cfazure"
	// "github.com/yomorun/yomo/pkg/bridge/ai/provider/cfopenai"
	// "github.com/yomorun/yomo/pkg/bridge/ai/provider/gemini"
	// "github.com/yomorun/yomo/pkg/bridge/ai/provider/openai"
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

		// log.InfoStatusEvent(os.Stdout, "")
		ylog.Info("Starting Zipper...")
		// config
		conf, err := pkgconfig.ParseConfigFile(config)
		if err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
			return
		}
		ctx := context.Background()
		// listening address.
		listenAddr := fmt.Sprintf("%s:%d", conf.Host, conf.Port)

		options := []yomo.ZipperOption{}
		tokenString := ""
		if _, ok := conf.Auth["type"]; ok {
			if tokenString, ok = conf.Auth["token"]; ok {
				options = append(options, yomo.WithAuth("token", tokenString))
			}
		}
		// check llm bridge server config
		// parse the llm bridge config
		bridgeConf := conf.Bridge
		aiConfig, err := ai.ParseConfig(bridgeConf)
		if err != nil {
			if err == ai.ErrConfigNotFound {
				log.InfoStatusEvent(os.Stdout, err.Error())
			} else {
				log.FailureStatusEvent(os.Stdout, err.Error())
				return
			}
		}
		if aiConfig != nil {
			// add AI connection middleware
			options = append(options, yomo.WithZipperConnMiddleware(ai.ConnMiddleware))
		}
		// new zipper
		zipper, err := yomo.NewZipper(
			conf.Name,
			router.Default(),
			nil,
			conf.Mesh,
			options...)
		if err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
			return
		}
		zipper.Logger().Info("using config file", "file_path", config)

		// AI Server
		if aiConfig != nil {
			// register the llm provider
			registerAIProvider(aiConfig)
			// start the llm api server
			go func() {
				err := ai.Serve(aiConfig, listenAddr, fmt.Sprintf("token:%s", tokenString))
				if err != nil {
					log.FailureStatusEvent(os.Stdout, err.Error())
					return
				}
			}()
		}

		// start the zipper
		err = zipper.ListenAndServe(ctx, listenAddr)
		if err != nil {
			log.FailureStatusEvent(os.Stdout, err.Error())
			return
		}
	},
}

func registerAIProvider(aiConfig *ai.Config) error {
	for name, provider := range aiConfig.Providers {
		switch name {
		case "azopenai":
			ai.RegisterProvider(azopenai.NewProvider(
				provider["api_key"],
				provider["api_endpoint"],
				provider["deployment_id"],
				provider["api_version"],
			))
		// case "gemini":
		// 	ai.RegisterProvider(gemini.NewProvider(provider["api_key"]))
		// case "openai":
		// 	ai.RegisterProvider(openai.NewProvider(provider["api_key"], provider["model"]))
		// case "cloudflare_azure":
		// 	ai.RegisterProvider(cfazure.NewProvider(
		// 		provider["endpoint"],
		// 		provider["api_key"],
		// 		provider["resource"],
		// 		provider["deployment_id"],
		// 		provider["api_version"],
		// 	))
		// case "cloudflare_openai":
		// 	ai.RegisterProvider(cfopenai.NewProvider(
		// 		provider["endpoint"],
		// 		provider["api_key"],
		// 		provider["model"],
		// 	))
		default:
			log.WarningStatusEvent(os.Stdout, "unknown provider: %s", name)
		}
	}

	// log.InfoStatusEvent(os.Stdout, "registered [%d] AI provider", len(ai.ListProviders()))
	ylog.Info("registered AI providers", "len", len(ai.ListProviders()))
	return nil
}

func init() {
	rootCmd.AddCommand(serveCmd)

	serveCmd.Flags().StringVarP(&config, "config", "c", "", "config file")
}
