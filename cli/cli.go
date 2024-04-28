// Command-line tools for YoMo

package cli

import (
	"fmt"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/yomorun/yomo/cli/serverless"
	"github.com/yomorun/yomo/pkg/file"
)

// defaultSFNFile is the default serverless file name
const (
	defaultSFNSourceFile     = "app.go"
	defaultSFNTestSourceFile = "app_test.go"
	defaultSFNCompliedFile   = "sfn.yomo"
	defaultSFNWASIFile       = "sfn.wasm"
)

// GetRootPath get root path
func GetRootPath() string {
	_, filename, _, ok := runtime.Caller(0)
	if ok {
		return path.Dir(filename)
	}
	return ""
}

func parseZipperAddr(opts *serverless.Options) error {
	url := opts.ZipperAddr
	if url == "" {
		opts.ZipperAddr = "localhost:9000"
		return nil
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
	return "YOMO_SFN_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

func bindViper(cmd *cobra.Command) *viper.Viper {
	v := viper.New()

	// bind environment variables
	// v.AllowEmptyEnv(true)
	v.SetEnvPrefix("YOMO_SFN")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.BindPFlags(cmd.Flags())
	v.AutomaticEnv()
	return v
}

func loadOptionsFromViper(runViper *viper.Viper, opts *serverless.Options) {
	opts.Name = runViper.GetString("name")
	opts.ZipperAddr = runViper.GetString("zipper")
	opts.Credential = runViper.GetString("credential")
	opts.ModFile = runViper.GetString("modfile")
	opts.Runtime = runViper.GetString("runtime")
	opts.WASI = runViper.GetBool("wasi")
}

func parseFileArg(args []string, opts *serverless.Options, defaultFiles ...string) error {
	if len(args) >= 1 && args[0] != "" {
		opts.Filename = args[0]
		return checkOptions(opts)
	}
	for _, f := range defaultFiles {
		opts.Filename = f
		err := checkOptions(opts)
		if err == nil {
			break
		}
	}
	return nil
}

func checkOptions(opts *serverless.Options) error {
	f, err := filepath.Abs(opts.Filename)
	if err != nil {
		return err
	}
	if !file.Exists(f) {
		return fmt.Errorf("file %s not found", f)
	}
	opts.Filename = f
	return nil
}
