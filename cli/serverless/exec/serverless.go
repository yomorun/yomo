package exec

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/yomorun/yomo/cli/serverless"
	"github.com/yomorun/yomo/pkg/file"
	"github.com/yomorun/yomo/pkg/log"
)

// ExecServerless defines executable file implementation of Serverless interface.
type ExecServerless struct {
	target string
}

// Init initializes the serverless
func (s *ExecServerless) Init(opts *serverless.Options) error {
	if !file.Exists(opts.Filename) {
		return fmt.Errorf("the file %s doesn't exist", opts.Filename)
	}
	s.target = opts.Filename

	return nil
}

// Build compiles the serverless to executable
func (s *ExecServerless) Build(clean bool) error {
	return nil
}

// Run compiles and runs the serverless
func (s *ExecServerless) Run(verbose bool) error {
	log.InfoStatusEvent(os.Stdout, "Run: %s", s.target)
	dir := file.Dir(s.target)
	cmd := exec.Command(s.target)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Args = os.Args
	cmd.Dir = dir
	err := serverless.LoadEnvFile(dir)
	if err != nil {
		return err
	}
	env := os.Environ()
	if verbose {
		cmd.Env = append(env, "YOMO_LOG_LEVEL=debug")
	}
	cmd.Env = env
	return cmd.Run()
}

// Executable returns true if the serverless is executable
func (s *ExecServerless) Executable() bool {
	return true
}

func init() {
	serverless.Register(&ExecServerless{}, ".yomo", ".exe")
}
