// Command-line tools for YoMo

package cli

import (
	"fmt"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/spf13/viper"
	"github.com/yomorun/yomo/cli/serverless"
	"github.com/yomorun/yomo/pkg/file"
)

// defaultSFNFile is the default serverless file name
const (
	defaultSFNSourceFile       = "app.go"
	defaultSFNSourceTSFile     = "src/app.ts"
	defaultSFNTestSourceFile   = "app_test.go"
	defaultSFNTestSourceTSFile = "app_test.ts"
	defaultSFNCompliedFile     = "sfn.yomo"
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

// loadOptionsFromViper load options from viper, supports flags and environment variables
func loadOptionsFromViper(v *viper.Viper, opts *serverless.Options) {
	opts.Name = v.GetString("name")
	opts.ZipperAddr = v.GetString("zipper")
	opts.Credential = v.GetString("credential")
	opts.ModFile = v.GetString("modfile")
	opts.Runtime = v.GetString("runtime")
}

func parseFileArg(opts *serverless.Options, defaultFiles ...string) error {
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

// DefaultSFNSourceFile returns the default source file name for the given runtime
func DefaultSFNSourceFile(runtime string) string {
	switch runtime {
	case "go": // go
		return defaultSFNSourceFile
	default: // node
		return defaultSFNSourceTSFile
	}
}

// DefaultSFNTestSourceFile returns the default test source file name
func DefaultSFNTestSourceFile(runtime string) string {
	switch runtime {
	case "go": // go
		return defaultSFNTestSourceFile
	default: // node
		return defaultSFNTestSourceTSFile
	}
}
