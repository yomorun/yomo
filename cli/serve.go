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
	"github.com/yomorun/yomo/core/auth"
	"github.com/yomorun/yomo/core/ylog"
	pkgconfig "github.com/yomorun/yomo/pkg/config"
	"github.com/yomorun/yomo/pkg/listener/mem"
	"github.com/yomorun/yomo/pkg/log"
	"github.com/yomorun/yomo/pkg/trace"

	"github.com/yomorun/yomo/pkg/bridge/ai"
	providerpkg "github.com/yomorun/yomo/pkg/bridge/ai/provider"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider/anthropic"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider/azopenai"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider/cerebras"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider/cfazure"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider/cfopenai"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider/gemini"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider/githubmodels"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider/ollama"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider/openai"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider/vertexai"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider/vllm"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider/xai"
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
		ylog.Info("Starting YoMo Zipper...")
		// config
		conf, err := pkgconfig.ParseConfigFile(config)
		if err != nil {
			log.FailureStatusEvent(os.Stdout, "%s", err.Error())
			return
		}

		trace.SetTracerProvider()

		ctx := context.Background()
		// listening address.
		listenAddr := fmt.Sprintf("%s:%d", conf.Host, conf.Port)

		// memory listener
		var listener *mem.Listener

		options := []yomo.ZipperOption{}
		tokenString := ""
		if _, ok := conf.Auth["type"]; ok {
			if tokenString, ok = conf.Auth["token"]; ok {
				options = append(options, yomo.WithAuth("token", tokenString))
			}
		}

		// check and parse the llm bridge server config
		bridgeConf := conf.Bridge
		aiConfig, err := ai.ParseConfig(bridgeConf)
		if err != nil {
			if err == ai.ErrConfigNotFound {
				log.InfoStatusEvent(os.Stdout, "%s", err.Error())
			} else {
				log.FailureStatusEvent(os.Stdout, "%s", err.Error())
				return
			}
		}
		if aiConfig != nil {
			listener = mem.Listen()
			// add AI connection middleware
			options = append(options, yomo.WithFrameListener(listener))
		}
		// new zipper
		zipper, err := yomo.NewZipper(
			conf.Name,
			conf.Mesh,
			options...)
		if err != nil {
			log.FailureStatusEvent(os.Stdout, "%s", err.Error())
			return
		}
		zipper.Logger().Info("using config file", "file_path", config)

		// AI Server
		if aiConfig != nil {
			// register the llm provider
			registerAIProvider(aiConfig)
			// start the llm api server
			go func() {
				conn, _ := listener.Dial()
				source := ai.NewSource(conn, auth.NewCredential(fmt.Sprintf("token:%s", tokenString)))

				conn2, _ := listener.Dial()
				reducer := ai.NewReducer(conn2, auth.NewCredential(fmt.Sprintf("token:%s", tokenString)))

				err := ai.Serve(aiConfig, ylog.Default(), source, reducer)
				if err != nil {
					log.FailureStatusEvent(os.Stdout, "%s", err.Error())
					return
				}
			}()
		}

		// start the zipper
		err = zipper.ListenAndServe(ctx, listenAddr)
		if err != nil {
			log.FailureStatusEvent(os.Stdout, "%s", err.Error())
			return
		}
	},
}

func registerAIProvider(aiConfig *ai.Config) error {
	for name, provider := range aiConfig.Providers {
		switch name {
		case "azopenai":
			providerpkg.RegisterProvider(azopenai.NewProvider(
				provider["api_key"],
				provider["api_endpoint"],
				provider["deployment_id"],
				provider["api_version"],
			))
		case "openai":
			providerpkg.RegisterProvider(openai.NewProvider(provider["api_key"], provider["model"]))
		case "cloudflare_azure":
			providerpkg.RegisterProvider(cfazure.NewProvider(
				provider["endpoint"],
				provider["api_key"],
				provider["resource"],
				provider["deployment_id"],
				provider["api_version"],
			))
		case "cloudflare_openai":
			providerpkg.RegisterProvider(cfopenai.NewProvider(
				provider["endpoint"],
				provider["api_key"],
				provider["model"],
			))
		case "ollama":
			providerpkg.RegisterProvider(ollama.NewProvider(provider["api_endpoint"], provider["model"]))
		case "gemini":
			providerpkg.RegisterProvider(gemini.NewProvider(provider["api_key"]))
		case "githubmodels":
			providerpkg.RegisterProvider(githubmodels.NewProvider(provider["api_key"], provider["model"]))
		case "cerebras":
			providerpkg.RegisterProvider(cerebras.NewProvider(provider["api_key"], provider["model"]))
		case "anthropic":
			providerpkg.RegisterProvider(anthropic.NewProvider(provider["api_key"], provider["model"]))
		case "xai":
			providerpkg.RegisterProvider(xai.NewProvider(provider["api_key"], provider["model"]))
		case "vertexai":
			providerpkg.RegisterProvider(vertexai.NewProvider(
				provider["project_id"],
				provider["location"],
				provider["model"],
				provider["credentials_file"],
			))
		case "deepseek":
			providerpkg.RegisterProvider(cerebras.NewProvider(provider["api_key"], provider["model"]))
		case "vllm":
			providerpkg.RegisterProvider(vllm.NewProvider(provider["api_endpoint"], provider["api_key"], provider["model"]))
		default:
			log.WarningStatusEvent(os.Stdout, "unknown provider: %s", name)
		}
	}

	ylog.Info("register LLM providers", "num", len(providerpkg.ListProviders()))
	return nil
}

func init() {
	rootCmd.AddCommand(serveCmd)

	serveCmd.Flags().StringVarP(&config, "config", "c", "", "config file")
}
