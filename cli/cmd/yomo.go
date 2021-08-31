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
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yomorun/yomo/cli/serverless"
	"github.com/yomorun/yomo/logger"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/viper"
)

var (
	config  string
	url     string
	opts    serverless.Options
	verbose bool
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:     "yomo",
	Version: GetVersion(),
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if verbose {
			logger.EnableDebug()
		}
	},
	// Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	cobra.OnInitialize(initConfig)

	// set version
	setVersion()

	// overwrite the shorthand of version flag to V.
	rootCmd.Flags().BoolP("version", "V", false, "version for yomo")

	// set verbose flag
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/yomo.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	// rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if config != "" {
		// Use config file from the flag.
		viper.SetConfigFile(config)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		cobra.CheckErr(err)

		// Search config in home directory with name ".cli" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName("yomo")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}

func setVersion() {
	template := fmt.Sprintf("YoMo CLI version: %s\n", rootCmd.Version)
	rootCmd.SetVersionTemplate(template)
}
