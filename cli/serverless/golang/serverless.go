package golang

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/yomorun/yomo/cli/serverless"
	"github.com/yomorun/yomo/pkg/file"
	"github.com/yomorun/yomo/pkg/log"
	"golang.org/x/mod/modfile"
	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/imports"
)

// GolangServerless defines golang implementation of Serverless interface.
type GolangServerless struct {
	opts    *serverless.Options
	source  string
	target  string
	tempDir string
}

// Init initializes the serverless
func (s *GolangServerless) Init(opts *serverless.Options) error {
	// now := time.Now()
	// msg := "Init: serverless function..."
	// initSpinning := log.Spinner(os.Stdout, msg)
	// defer initSpinning(log.Failure)

	s.opts = opts
	if !file.Exists(s.opts.Filename) {
		return fmt.Errorf("the file %s doesn't exist", s.opts.Filename)
	}

	// generate source code
	source := file.GetBinContents(s.opts.Filename)
	if len(source) < 1 {
		return fmt.Errorf(`"%s" content is empty`, s.opts.Filename)
	}

	// append main function
	ctx := Context{
		Name:        s.opts.Name,
		ZipperAddrs: s.opts.ZipperAddrs,
		Credential:  s.opts.Credential,
		UseEnv:      s.opts.UseEnv,
	}

	// determine: rx stream serverless or raw bytes serverless.
	isRx := strings.Contains(string(source), "rx.Stream")
	mainFuncTmpl := ""
	if isRx {
		MainFuncRxTmpl = append(MainFuncRxTmpl, PartialsTmpl...)
		mainFuncTmpl = string(MainFuncRxTmpl)
	} else {
		MainFuncRawBytesTmpl = append(MainFuncRawBytesTmpl, PartialsTmpl...)
		mainFuncTmpl = string(MainFuncRawBytesTmpl)
	}

	mainFunc, err := RenderTmpl(mainFuncTmpl, &ctx)
	if err != nil {
		return fmt.Errorf("Init: %s", err)
	}
	source = append(source, mainFunc...)
	// log.InfoStatusEvent(os.Stdout, "merge source elapse: %v", time.Since(now))
	// Create the AST by parsing src
	// fmt.Println(string(source))
	fset := token.NewFileSet()
	astf, err := parser.ParseFile(fset, "", source, 0)
	if err != nil {
		return fmt.Errorf("Init: parse source file err %s", err)
	}
	// Add import packages
	astutil.AddNamedImport(fset, astf, "", "github.com/yomorun/yomo")
	astutil.AddNamedImport(fset, astf, "", "github.com/joho/godotenv")
	// log.InfoStatusEvent(os.Stdout, "import elapse: %v", time.Since(now))
	// Generate the code
	code, err := generateCode(fset, astf)
	if err != nil {
		return fmt.Errorf("Init: generate code err %s", err)
	}
	// Create a temp folder.
	tempDir, err := os.MkdirTemp("", "yomo_")
	if err != nil {
		return err
	}
	s.tempDir = tempDir
	tempFile := filepath.Join(tempDir, "app.go")
	// Fix imports
	fixedSource, err := imports.Process(tempFile, code, nil)
	if err != nil {
		return fmt.Errorf("Init: imports %s", err)
	}
	// log.InfoStatusEvent(os.Stdout, "fix import elapse: %v", time.Since(now))
	if err := file.PutContents(tempFile, fixedSource); err != nil {
		return fmt.Errorf("Init: write file err %s", err)
	}
	// log.InfoStatusEvent(os.Stdout, "final write file elapse: %v", time.Since(now))
	// mod
	name := strings.ReplaceAll(opts.Name, " ", "_")
	if name == "" {
		name = "yomo-sfn"
	}
	cmd := exec.Command("go", "mod", "init", name)
	cmd.Dir = tempDir
	env := os.Environ()
	env = append(env, fmt.Sprintf("GO111MODULE=%s", "on"))
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("Init: go mod init err %s", out)
		return err
	}

	// TODO: check if is already built in temp dir by MD5
	s.source = tempFile
	return nil
}

// Build compiles the serverless to executable
func (s *GolangServerless) Build(clean bool) error {
	// check if the file exists
	appPath := s.source
	if _, err := os.Stat(appPath); os.IsNotExist(err) {
		return fmt.Errorf("the file %s doesn't exist", appPath)
	}
	// env
	env := os.Environ()
	env = append(env, fmt.Sprintf("GO111MODULE=%s", "on"))
	// use custom go.mod
	if s.opts.ModFile != "" {
		mfile, _ := filepath.Abs(s.opts.ModFile)
		if !file.Exists(mfile) {
			return fmt.Errorf("the mod file %s doesn't exist", mfile)
		}
		// go.mod
		log.WarningStatusEvent(os.Stdout, "Use custom go.mod: %s", mfile)
		modContent := file.GetBinContents(mfile)
		if len(modContent) == 0 {
			return errors.New("go.mod is empty")
		}
		f, err := modfile.Parse("go.mod", modContent, nil)
		if err != nil {
			return err
		}
		for _, r := range f.Replace {
			if strings.HasPrefix(r.New.Path, ".") {
				abs, err := filepath.Abs(r.New.Path)
				if err != nil {
					return err
				}
				modContent = bytes.Replace(modContent, []byte(r.New.Path), []byte(abs), 1)
			}
		}
		// fmt.Println(string(modContent))
		// wirte to temp go.mod
		tempMod := filepath.Join(s.tempDir, "go.mod")
		if err := file.PutContents(tempMod, modContent); err != nil {
			return fmt.Errorf("write go.mod err %s", err)
		}
		// mod download
		cmd := exec.Command("go", "mod", "tidy")
		cmd.Env = env
		cmd.Dir = s.tempDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			err = fmt.Errorf("Build: go mod tidy err %s", out)
			return err
		}
	} else {
		// Upgrade modules that provide packages imported by packages in the main module
		cmd := exec.Command("go", "get", "-d", "./...")
		cmd.Dir = s.tempDir
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			err = fmt.Errorf("Build: go get err %s", out)
			return err
		}
	}
	// build
	goos := runtime.GOOS
	dir, _ := filepath.Split(s.opts.Filename)
	sl, _ := filepath.Abs(dir + "sl.yomo")

	// clean build
	if clean {
		defer func() {
			file.Remove(s.tempDir)
		}()
	}
	s.target = sl
	// fmt.Printf("goos=%s\n", goos)
	if goos == "windows" {
		sl, _ = filepath.Abs(dir + "sl.exe")
		s.target = sl
	}
	// go build
	cmd := exec.Command("go", "build", "-ldflags", "-s -w", "-o", sl, appPath)
	cmd.Env = env
	cmd.Dir = s.tempDir
	// log.InfoStatusEvent(os.Stdout, "Build: cmd: %+v", cmd)
	// source := file.GetContents(s.source)
	// log.InfoStatusEvent(os.Stdout, "source: %s", source)
	out, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("Build: failure %s", out)
		return err
	}
	return nil
}

// Run compiles and runs the serverless
func (s *GolangServerless) Run(verbose bool) error {
	log.InfoStatusEvent(os.Stdout, "Run: %s", s.target)
	cmd := exec.Command(s.target)
	if verbose {
		cmd.Env = []string{"YOMO_LOG_LEVEL=debug"}
	}
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	return cmd.Run()
}

func (s *GolangServerless) Executable() bool {
	return false
}

func generateCode(fset *token.FileSet, file *ast.File) ([]byte, error) {
	var output []byte
	buffer := bytes.NewBuffer(output)
	if err := printer.Fprint(buffer, fset, file); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

func init() {
	serverless.Register(&GolangServerless{}, ".go")
}
