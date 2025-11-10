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
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/auth"
	"github.com/yomorun/yomo/core/ylog"
	pkgconfig "github.com/yomorun/yomo/pkg/config"
	"github.com/yomorun/yomo/pkg/listener/mem"
	"github.com/yomorun/yomo/pkg/log"
	"github.com/yomorun/yomo/pkg/trace"

	"github.com/yomorun/yomo/pkg/bridge/ai"
	providerpkg "github.com/yomorun/yomo/pkg/bridge/ai/provider"
	"github.com/yomorun/yomo/pkg/bridge/llm"
	"github.com/yomorun/yomo/pkg/bridge/mcp"
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
		// check and parse the mcp server config
		mcpConfig, err := mcp.ParseConfig(bridgeConf)
		if err != nil {
			if err == mcp.ErrConfigNotFound {
				ylog.Warn("mcp server is disabled")
			} else {
				log.FailureStatusEvent(os.Stdout, "%s", err.Error())
				return
			}
		}

		options = append(options, yomo.WithZipperFrameMiddleware(core.RejectReservedTagMiddleware))

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

		if aiConfig != nil {
			// AI Server
			// register the llm provider
			registerAIProvider(aiConfig)
			// start the llm api server
			go func() {
				conn, _ := listener.Dial()
				source := ai.NewReduceSource(conn, auth.NewCredential(fmt.Sprintf("token:%s", tokenString)))

				conn2, _ := listener.Dial()
				reducer := ai.NewReducer(conn2, auth.NewCredential(fmt.Sprintf("token:%s", tokenString)))

				err := llm.Serve(aiConfig, ylog.Default(), source, reducer)
				if err != nil {
					log.FailureStatusEvent(os.Stdout, "%s", err.Error())
					return
				}
			}()
			// MCP Server
			if mcpConfig != nil {
				conn, _ := listener.Dial()
				source := ai.NewReduceSource(conn, auth.NewCredential(fmt.Sprintf("token:%s", tokenString)))

				conn2, _ := listener.Dial()
				reducer := ai.NewReducer(conn2, auth.NewCredential(fmt.Sprintf("token:%s", tokenString)))

				go func() {
					defer mcp.Stop()

					err = mcp.Start(mcpConfig, aiConfig, source, reducer, ylog.Default())
					if err != nil {
						log.FailureStatusEvent(os.Stdout, "%s", err.Error())
						return
					}
				}()
			}
		}

		// start the zipper
		err = zipper.ListenAndServe(ctx, listenAddr)
		if err != nil {
			log.FailureStatusEvent(os.Stdout, "%s", err.Error())
			return
		}
	},
}

func registerAIProvider(aiConfig *ai.Config) {
	for name, provider := range aiConfig.Providers {
		p, err := ai.NewProviderFromConfig(name, provider)
		if err != nil {
			log.WarningStatusEvent(os.Stdout, "%s", err.Error())
		} else {
			providerpkg.RegisterProvider(p)
		}
	}
	ylog.Info("register LLM providers", "num", len(providerpkg.ListProviders()))
}

func init() {
	rootCmd.AddCommand(serveCmd)

	serveCmd.Flags().StringVarP(&config, "config", "c", "", "config file")
}
