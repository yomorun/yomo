// Command-line tools for YoMo

package cli

import (
	"fmt"
	"path"
	"runtime"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/yomorun/yomo/cli/serverless"
	"github.com/yomorun/yomo/pkg/file"
)

// defaultSFNFile is the default serverless file name
const (
	defaultSFNSourceFile     = "app.go"
	defaultSFNSTestourceFile = "app_test.go"
	defaultSFNCompliedFile   = "sfn.wasm"
)

// GetRootPath get root path
func GetRootPath() string {
	_, filename, _, ok := runtime.Caller(0)
	if ok {
		return path.Dir(filename)
	}
	return ""
}

func parseURL(url string, opts *serverless.Options) error {
	url = strings.TrimSpace(url)
	if url == "" {
		url = "localhost:9000"
	}

	splits := strings.Split(url, ":")
	if len(splits) != 2 {
		return fmt.Errorf(`the format of url "%s" is incorrect, it should be "host:port", e.g. localhost:9000`, url)
	}

	port, err := strconv.Atoi(splits[1])
	if err != nil {
		return fmt.Errorf("%s: invalid port: %s", url, splits[1])
	}

	opts.ZipperAddr = fmt.Sprintf("%s:%d", splits[0], port)

	return nil
}

func getViperName(name string) string {
	return "yomo_sfn_" + strings.ReplaceAll(name, "-", "_")
}

func bindViper(cmd *cobra.Command) *viper.Viper {
	v := viper.New()

	// bind environment variables
	v.AllowEmptyEnv(true)
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		name := getViperName(f.Name)
		v.BindEnv(name)
		v.SetDefault(name, f.DefValue)
	})

	return v
}

func loadViperValue(cmd *cobra.Command, v *viper.Viper, p *string, name string) {
	f := cmd.Flag(name)
	if !f.Changed {
		*p = v.GetString(getViperName(name))
	}
}

func parseFileArg(args []string, opts *serverless.Options, defaultFile string) error {
	if len(args) >= 1 && args[0] != "" {
		opts.Filename = args[0]
	} else {
		opts.Filename = defaultFile
	}
	if !file.Exists(opts.Filename) {
		return fmt.Errorf("file %s not found", opts.Filename)
	}
	return nil
}
