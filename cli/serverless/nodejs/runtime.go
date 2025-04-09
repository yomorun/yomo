package nodejs

import (
	"errors"
	"fmt"
	"html/template"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

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
	outputDir    string // eg. dist/

	// command path
	nodePath string
	npmPath  string
}

// NewWrapper returns a new NodejsWrapper
func NewWrapper(functionName, entryTSFile string) (*NodejsWrapper, error) {
	// check node
	nodePath, err := exec.LookPath("node")
	if err != nil {
		return nil, errors.New("the Node.js runtime is not found. To run TypeScript serverless functions, please install Node.js from https://nodejs.org")
	}

	// prefer pnpm, if pnpm is not found, fallback to npm
	npmPath, err := exec.LookPath("pnpm")
	if err != nil {
		npmPath, _ = exec.LookPath("npm")
	}

	// check entry file
	ext := filepath.Ext(entryTSFile)
	if ext != ".ts" {
		return nil, fmt.Errorf("this runtime only supports Typescript files (.ts), got: %s", entryTSFile)
	}

	// set workdir
	// workdir := filepath.Dir(entryTSFile)
	workdir := "./"

	// set output dir
	outputDir := filepath.Join(workdir, "dist")

	// the compiled js file
	entryJSFile := entryTSFile[:len(entryTSFile)-len(ext)] + ".js"

	// the file name without extension
	fileName := entryTSFile[:len(entryTSFile)-len(ext)]

	w := &NodejsWrapper{
		functionName: functionName,
		workDir:      workdir,
		entryTSFile:  entryTSFile,
		entryJSFile:  entryJSFile,
		fileName:     fileName,
		nodePath:     nodePath,
		npmPath:      npmPath,
		outputDir:    outputDir,
	}

	return w, nil
}

// WorkDir returns the working directory of the serverless function to build and run.
func (w *NodejsWrapper) WorkDir() string {
	return w.workDir
}

// Build defines how to build the serverless llm function.
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

	// 3. check tsconfig.json exist
	tsconfigPath := filepath.Join(w.workDir, "tsconfig.json")
	if _, err := os.Stat(tsconfigPath); os.IsNotExist(err) {
		// not exist, create it using tsc --init
		cmdTSCInit := exec.Command("tsc", "--init", "--outDir", "./dist")
		cmdTSCInit.Dir = w.workDir
		cmdTSCInit.Stdout = os.Stdout
		cmdTSCInit.Stderr = os.Stderr
		cmdTSCInit.Env = env
		if err := cmdTSCInit.Run(); err != nil {
			return err
		}
	}

	// 4. check tsconfig include
	tsconfigData, err := os.ReadFile(tsconfigPath)
	if err != nil {
		return fmt.Errorf("failed to read tsconfig.json: %v", err)
	}
	includePath := gjson.GetBytes(tsconfigData, "include")
	if !includePath.Exists() {
		// "include" doesn't exist, add it with .wrapper.ts
		tsconfigData, err = sjson.SetBytes(tsconfigData, "include", []string{wrapperTS})
		if err != nil {
			return fmt.Errorf("failed to modify tsconfig.json: %v", err)
		}
	} else {
		// "include" exists, check if .wrapper.ts is already included
		includeArray := []string{}
		for _, item := range includePath.Array() {
			includeArray = append(includeArray, item.String())
		}
		includeFound := false
		for _, item := range includeArray {
			if item == wrapperTS {
				includeFound = true
				break
			}
		}
		// if .wrapper.ts isn't found in the include array, append it
		if !includeFound {
			includeArray = append(includeArray, wrapperTS)
			tsconfigData, err = sjson.SetBytes(tsconfigData, "include", includeArray)
			if err != nil {
				return fmt.Errorf("failed to modify tsconfig.json: %v", err)
			}
		}
	}
	if err := os.WriteFile(tsconfigPath, tsconfigData, 0644); err != nil {
		return fmt.Errorf("failed to write tsconfig.json: %v", err)
	}

	// 5. compile ts file to js
	cmd2 := exec.Command("tsc")
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
	// try to run with bunjs
	// first, check if bun is installed
	bunPath, err := exec.LookPath("bun")
	if err == nil {
		// bun is installed, run the wrapper with bun
		log.Println("Bun is installed, bun --version:")
		cmd := exec.Command(bunPath, "--version")
		cmd.Dir = w.workDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		cmd.Run()

		cmd = exec.Command(bunPath, wrapperTS)
		cmd.Dir = w.workDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = env

		return cmd.Run()
	} else {
		log.Println("Bun is not installed, check Nodejs")
	}

	// if bun is not found, fallback to nodejs
	cmd := exec.Command(w.nodePath, filepath.Join(w.outputDir, wrapperJS))
	cmd.Dir = w.workDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env

	return cmd.Run()
}

func (w *NodejsWrapper) genWrapperTS(functionName, dstPath string) error {
	baseFilename := w.workDir + filepath.Base(w.fileName)
	entryTS := baseFilename + ".ts"

	data := struct {
		WorkDir      string
		FunctionName string
		FileName     string
		FilePath     string
	}{
		WorkDir:      w.workDir,
		FunctionName: functionName,
		FileName:     baseFilename,
		FilePath:     entryTS,
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

// InitApp initializes the nodejs application
func (w *NodejsWrapper) InitApp() error {
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
	cmd.Dir = w.workDir
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("run %s failed: %v", cmd.String(), err)
	}
	return nil
}
