package nodejs

import (
	"errors"
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"path/filepath"

	_ "embed"
)

//go:embed templates/wrapper_ts.tmpl
var WrapperTSTmpl string

var (
	wrapperTS = ".wrapper.ts"
	wrapperJS = ".wrapper.js"
)

// NodejsWrapper is the nodejs implementation of wrapper.
type NodejsWrapper struct {
	functionName string
	workDir      string // eg. src/
	entryTSFile  string // eg. src/app.ts
	entryJSFile  string // eg. src/app.js
	fileName     string // eg. src/app

	// command path
	nodePath string
	npmPath  string
}

// NewWrapper returns a new NodejsWrapper
func NewWrapper(functionName, entryTSFile string) (*NodejsWrapper, error) {
	// check command
	nodePath, err := exec.LookPath("node")
	if err != nil {
		return nil, errors.New("[node] command was not found. to run the sfn in ts, you need to install node. For details, visit https://nodejs.org")
	}
	npmPath, err := exec.LookPath("pnpm")
	if err != nil {
		npmPath, _ = exec.LookPath("npm")
	}

	ext := filepath.Ext(entryTSFile)
	if ext != ".ts" {
		return nil, fmt.Errorf("only support typescript, got: %s", entryTSFile)
	}
	workdir := filepath.Dir(entryTSFile)

	entryJSFile := entryTSFile[:len(entryTSFile)-len(ext)] + ".js"

	fileName := entryTSFile[:len(entryTSFile)-len(filepath.Ext(entryTSFile))]

	w := &NodejsWrapper{
		functionName: functionName,
		workDir:      workdir,
		entryTSFile:  entryTSFile,
		entryJSFile:  entryJSFile,
		fileName:     fileName,
		nodePath:     nodePath,
		npmPath:      npmPath,
	}

	return w, nil
}

// WorkDir returns the working directory of the serverless function to build and run.
func (w *NodejsWrapper) WorkDir() string {
	return w.workDir
}

// Build defines how to build the serverless function.
func (w *NodejsWrapper) Build(env []string) error {
	// 1. generate .wrapper.ts file
	dstPath := filepath.Join(w.workDir, wrapperTS)
	_ = os.Remove(dstPath)

	if err := w.genWrapperTS(w.functionName, dstPath); err != nil {
		return err
	}

	// 2. install dependencies
	cmd := exec.Command(w.npmPath, "install")
	cmd.Dir = w.workDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env
	if err := cmd.Run(); err != nil {
		return err
	}

	// 3. compile .wrapper.ts file to .wrapper.js
	cmd2 := exec.Command("tsc", wrapperTS)
	cmd2.Dir = w.workDir
	cmd2.Stdout = os.Stdout
	cmd2.Stderr = os.Stderr
	cmd2.Env = env
	if err := cmd2.Run(); err != nil {
		return err
	}

	return nil
}

// Run runs the serverless function
func (w *NodejsWrapper) Run(env []string) error {
	cmd := exec.Command(w.nodePath, wrapperJS)
	cmd.Dir = w.workDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env

	return cmd.Run()
}

func (w *NodejsWrapper) genWrapperTS(functionName, dstPath string) error {
	data := struct {
		WorkDir      string
		FunctionName string
		FileName     string
		FilePath     string
	}{
		WorkDir:      w.workDir,
		FunctionName: functionName,
		FileName:     w.fileName,
		FilePath:     w.entryTSFile,
	}

	dst, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE, 0755)
	if err != nil {
		return err
	}
	defer dst.Close()

	t := template.Must(template.New("wrapper").Parse(WrapperTSTmpl))
	if err := t.Execute(dst, data); err != nil {
		return err
	}

	return nil
}

// Init initializes the nodejs application
func (w *NodejsWrapper) Init() error {
	// init
	cmd := exec.Command(w.npmPath, "init")
	if w.npmPath == "npm" {
		cmd.Args = append(cmd.Args, "-y")
	}
	cmd.Dir = w.workDir
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run %s failed: %v", cmd.String(), err)
	}
	return nil
}

// InstallDeps installs the yomo dependencies
func (w *NodejsWrapper) InstallDeps() error {
	// @yomo/sfn
	cmd := exec.Command(w.npmPath, "install", "@yomo/sfn")
	cmd.Dir = w.workDir
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("run %s failed: %v", cmd.String(), err)
	}
	// devDependencies
	cmd = exec.Command(w.npmPath, "install", "-D", "@types/node", "ts-node")
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("run %s failed: %v", cmd.String(), err)
	}
	return nil
}
