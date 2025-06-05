// Package nodejs provides a ts serverless runtime
package nodejs

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/yomorun/yomo/cli/serverless"
	"github.com/yomorun/yomo/pkg/file"
	"github.com/yomorun/yomo/pkg/log"
	"github.com/yomorun/yomo/pkg/wrapper"
)

// nodejsServerless will start js program to run serverless functions.
type nodejsServerless struct {
	name       string
	zipperAddr string
	credential string
	wrapper    *NodejsWrapper
}

// Setup sets up the nodejs serverless
func (s *nodejsServerless) Setup(opts *serverless.Options) error {
	wrapper, err := NewWrapper(opts.Name, opts.Filename)
	if err != nil {
		return err
	}

	// create work dir
	err = file.Mkdir(wrapper.workDir)
	if err != nil {
		log.FailureStatusEvent(os.Stdout, "Create work dir failed: %v", err)
		return err
	}

	// generate tsconfig.json
	tsconfigPath := filepath.Join(wrapper.workDir, "tsconfig.json")
	if _, err := os.Stat(tsconfigPath); os.IsNotExist(err) {
		tsconfigContent := `{
  "compilerOptions": {
    "target": "esNext",
    "module": "commonjs",
    "forceConsistentCasingInFileNames": true,
    "strict": true,
    "outDir": "./dist",
    "rootDir": "./src",
    "skipLibCheck": true
  },
  "include": ["src/**/*"],
  "exclude": ["node_modules"]
}`
		if err := os.WriteFile(tsconfigPath, []byte(tsconfigContent), 0644); err != nil {
			return fmt.Errorf("failed to write tsconfig.json: %v", err)
		}
	}

	// generate package.json
	if !file.Exists(filepath.Join(wrapper.workDir, "package.json")) {
		err = wrapper.InitApp()
		if err != nil {
			return err
		}
	}

	// install dependencies
	err = wrapper.InstallDeps()
	if err != nil {
		return err
	}

	return nil
}

// Init initializes the nodejs serverless
func (s *nodejsServerless) Init(opts *serverless.Options) error {
	wrapper, err := NewWrapper(opts.Name, opts.Filename)
	if err != nil {
		return err
	}

	s.name = opts.Name
	s.zipperAddr = opts.ZipperAddr
	s.credential = opts.Credential
	s.wrapper = wrapper

	return nil
}

// Build calls wrapper.Build
func (s *nodejsServerless) Build(_ bool) error {
	return s.wrapper.Build(os.Environ())
}

// Run the wrapper.Run
func (s *nodejsServerless) Run(verbose bool) error {
	err := serverless.LoadEnvFile(s.wrapper.workDir)
	if err != nil {
		return err
	}
	env := os.Environ()
	if verbose {
		env = append(env, "YOMO_LOG_LEVEL=debug")
	}
	return wrapper.Run(s.name, s.zipperAddr, s.credential, s.wrapper, env)
}

// Executable shows whether the program needs to be built
func (s *nodejsServerless) Executable() bool {
	return true
}

func init() {
	serverless.Register(&nodejsServerless{}, ".ts")
}
