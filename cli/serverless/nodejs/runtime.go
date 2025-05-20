package nodejs

import (
	"errors"
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"path/filepath"

	_ "embed"

	"github.com/yomorun/yomo/pkg/log"
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
	workdir := filepath.Dir(filepath.Dir(entryTSFile))

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

	// 3. compile ts file to js
	cmd2 := exec.Command("tsc")
	cmd2.Dir = w.workDir
	cmd2.Stdout = os.Stdout
	cmd2.Stderr = os.Stderr
	cmd2.Env = env
	if err := cmd2.Run(); err != nil {
		return err
	}

	// 4. copy files other than .ts file from src/ to dist/src/ because tsc do not do that
	srcDir := filepath.Join(w.workDir, "src")
	dstDir := filepath.Join(w.workDir, "dist/src")
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return err
	}
	// copy all files from src/ to dist/
	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == srcDir {
			return nil
		}

		// Get relative path to maintain directory structure
		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %v", err)
		}

		if filepath.Ext(path) == ".ts" {
			return nil
		}

		// Create destination path with same structure
		dstPath := filepath.Join(dstDir, relPath)

		// Check if the destination directory exists
		dstDir := filepath.Dir(dstPath)
		if _, err := os.Stat(dstDir); os.IsNotExist(err) {
			return fmt.Errorf("destination directory %s does not exist", dstDir)
		}

		// Copy the file to ./dist/src/
		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		} else {
			// Read the source file
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("failed to read file %s: %v", path, err)
			}

			// Write to the destination file
			if err := os.WriteFile(dstPath, data, info.Mode()); err != nil {
				return fmt.Errorf("failed to write file %s: %v", dstPath, err)
			}

			log.InfoStatusEvent(os.Stdout, "copied %s to %s\n", path, dstPath)
			return nil
		}
	})

	return err
}

// Run runs the serverless function
func (w *NodejsWrapper) Run(env []string) error {
	// try to run with bunjs
	// first, check if bun is installed
	bunPath, err := exec.LookPath("bun")
	if err == nil {
		// bun is installed, run the wrapper with bun
		log.InfoStatusEvent(os.Stdout, "Bun version: %s\n", bunPath)
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
	baseFilename := "./src/" + filepath.Base(w.fileName)
	entryTS := baseFilename + ".ts"

	data := struct {
		WorkDir      string
		FunctionName string
		FileName     string
		FilePath     string
	}{
		WorkDir:      "./",
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

// InitApp initializes the nodejs application by `npm init -y`
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

	// add .gitignore file, and ignore node_modules/, dist/, .wrapper.ts
	gitignore := filepath.Join(w.workDir, ".gitignore")
	if _, err := os.Stat(gitignore); os.IsNotExist(err) {
		err = os.WriteFile(gitignore, []byte("node_modules/\ndist/\n.wrapper.ts\n"), 0644)
		if err != nil {
			return fmt.Errorf("write .gitignore failed: %v", err)
		}
	}

	return nil
}
