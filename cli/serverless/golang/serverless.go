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
	output  string
	tempDir string
}

// Init initializes the serverless
func (s *GolangServerless) Init(opts *serverless.Options) error {
	s.opts = opts
	if !file.Exists(s.opts.Filename) {
		return fmt.Errorf("the file %s doesn't exist", s.opts.Filename)
	}

	// generate source code
	source := file.GetBinContents(s.opts.Filename)
	if len(source) < 1 {
		return fmt.Errorf(`"%s" content is empty`, s.opts.Filename)
	}

	opt, err := ParseSrc(s.opts.Filename)
	if err != nil {
		return fmt.Errorf("parse source code: %s", err)
	}
	// append main function
	ctx := Context{
		Name:             s.opts.Name,
		ZipperAddr:       s.opts.ZipperAddr,
		Credential:       s.opts.Credential,
		UseEnv:           s.opts.UseEnv,
		WithInitFunc:     opt.WithInit,
		WithWantedTarget: opt.WithWantedTarget,
	}

	// determine: rx stream serverless or raw bytes serverless.
	isRx := strings.Contains(string(source), "rx.Stream")
	isWasm := true
	mainFuncTmpl := ""
	mainFunc, err := RenderTmpl(string(WasmMainFuncTmpl), &ctx)
	if err != nil {
		return fmt.Errorf("Init: %s", err)
	}
	if isRx {
		if isWasm {
			return errors.New("wasm does not support rx.Stream")
		}
		MainFuncRxTmpl = append(MainFuncRxTmpl, PartialsTmpl...)
		mainFuncTmpl = string(MainFuncRxTmpl)
		mainFunc, err = RenderTmpl(mainFuncTmpl, &ctx)
		if err != nil {
			return fmt.Errorf("Init: %s", err)
		}
	}

	source = append(source, mainFunc...)
	fset := token.NewFileSet()
	astf, err := parser.ParseFile(fset, "", source, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("Init: parse source file err %s", err)
	}
	// Add import packages
	astutil.AddNamedImport(fset, astf, "", "github.com/yomorun/yomo")
	astutil.AddNamedImport(fset, astf, "", "github.com/joho/godotenv")
	// wasm guest import
	astutil.AddNamedImport(fset, astf, "", "github.com/yomorun/yomo/serverless/guest")
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
	if err := file.PutContents(tempFile, fixedSource); err != nil {
		return fmt.Errorf("Init: write file err %s", err)
	}
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
	env = append(
		env,
		fmt.Sprintf("GO111MODULE=%s", "on"),
	)
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
	dir, _ := filepath.Split(s.opts.Filename)
	sl, _ := filepath.Abs(dir + "sfn.wasm")

	// clean build
	if clean {
		defer func() {
			file.Remove(s.tempDir)
		}()
	}
	s.output = sl
	tinygo, err := exec.LookPath("tinygo")
	if err != nil {
		return errors.New("[tinygo] command was not found. to build the wasm file, you need to install tinygo. For details, visit https://tinygo.org")
	}
	cmd := exec.Command(tinygo, "build", "-no-debug", "-target", "wasi", "-o", sl, appPath)
	cmd.Env = env
	cmd.Dir = s.tempDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("Build: failure, tinygo %s", out)
		return err
	}
	return nil
}

// Run compiles and runs the serverless
func (s *GolangServerless) Run(verbose bool) error {
	log.InfoStatusEvent(os.Stdout, "Run: %s", s.output)
	cmd := exec.Command(s.output)
	if verbose {
		cmd.Env = []string{"YOMO_LOG_LEVEL=debug", "YOMO_LOG_VERBOSE=true"}
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

type AppOpts struct {
	WithInit         bool
	WithWantedTarget bool
	WithDescription  bool
	WithInputSchema  bool
}

// ParseSrc parse app option from source code to run serverless
func ParseSrc(appFile string) (*AppOpts, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, appFile, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}

	opts := &AppOpts{}

	for _, v := range f.Decls {
		if d, ok := v.(*ast.FuncDecl); ok {
			switch d.Name.String() {
			case "Init":
				opts.WithInit = true
			case "Description":
				opts.WithDescription = true
			case "InputSchema":
				opts.WithInputSchema = true
			case "WantedTarget":
				opts.WithWantedTarget = true
			}
		}
	}

	return opts, nil
}

func init() {
	serverless.Register(&GolangServerless{}, ".go")
}
