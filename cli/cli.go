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
	if url == "" {
		url = "localhost:9000"
	}
	addrs := strings.Split(url, ",")
	for _, addr := range addrs {
		addr = strings.TrimSpace(addr)
		if len(addr) == 0 {
			continue
		}
		splits := strings.Split(addr, ":")
		l := len(splits)
		if l == 1 {
			opts.ZipperAddrs = append(opts.ZipperAddrs, splits[0]+":9000")
		} else if l == 2 {
			port, err := strconv.Atoi(splits[1])
			if err != nil {
				return fmt.Errorf("%s: invalid port: %s", addr, splits[1])
			}
			opts.ZipperAddrs = append(opts.ZipperAddrs, fmt.Sprintf("%s:%d", splits[0], port))
		} else {
			return fmt.Errorf(`the format of url "%s" is incorrect, it should be "host:port", f.e. localhost:9000`, addr)
		}
	}
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
