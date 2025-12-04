package nodejs

import (
	"errors"
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
	workDir     string // eg. src/
	entryTSFile string // eg. src/app.ts
	entryJSFile string // eg. src/app.js
	fileName    string // eg. src/app
	outputDir   string // eg. dist/

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
		workDir:     workdir,
		entryTSFile: entryTSFile,
		entryJSFile: entryJSFile,
		fileName:    fileName,
		nodePath:    nodePath,
		npmPath:     npmPath,
		outputDir:   outputDir,
	}

	return w, nil
}

// WorkDir returns the working directory of the serverless function to build and run.
func (w *NodejsWrapper) WorkDir() string {
	return w.workDir
}

// Build defines how to build the serverless llm function.
func (w *NodejsWrapper) Build(env []string) error {
	// 1. generate ./src/.wrapper.ts file
	dstPath := filepath.Join(w.workDir, "src/", wrapperTS)
	// remove the old one
	_ = os.Remove(dstPath)
	// create new one
	if err := w.genWrapperTS(dstPath); err != nil {
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

	// check if `tsgo` is installed, otherwise, use `tsc`
	tscCommand := "tsgo"
	_, err := exec.LookPath(tscCommand)
	if err != nil {
		tscCommand = "tsc"
		_, err = exec.LookPath(tscCommand)
		if err != nil {
			return fmt.Errorf("the TypeScript compiler (%s) is not found. Please install it with `npm install -g typescript`", tscCommand)
		}
	}

	// 3. compile ts files to js
	// get the version of tsgo/tsc
	var tscVersion string
	if v, err := checkVersion(tscCommand); err != nil {
		return err
	} else {
		tscVersion = v
	}
	log.InfoStatusEvent(os.Stdout, "Compiling with %s (%s)", tscCommand, tscVersion)

	cmd2 := exec.Command(tscCommand, "-p", "tsconfig.json")
	cmd2.Dir = w.workDir
	cmd2.Stdout = os.Stdout
	cmd2.Stderr = os.Stderr
	cmd2.Env = env
	if err := cmd2.Run(); err != nil {
		return err
	}

	// 4. copy src/app.ts to dist/app.ts
	baseFileName := filepath.Base(w.entryTSFile)
	srcEntryPath := w.entryTSFile
	dstEntryPath := filepath.Join(w.outputDir, baseFileName)

	data, err := os.ReadFile(srcEntryPath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %v", srcEntryPath, err)
	}

	if err := os.WriteFile(dstEntryPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write file %s: %v", dstEntryPath, err)
	}

	// log.InfoStatusEvent(os.Stdout, "Copied %s to %s", srcEntryPath, dstEntryPath)

	return err
}

// Run runs the serverless function
func (w *NodejsWrapper) Run(env []string) error {
	// ./dist/.wrapper.js
	entryJSFile := filepath.Join(w.outputDir, wrapperJS)
	// try to run with bunjs
	// first, check if bun is installed
	bunPath, err := exec.LookPath("bun")
	if err == nil {
		// bun is installed, run the wrapper with bun
		// get the version of bun
		var bunVersion string
		if v, err := checkVersion("bun"); err != nil {
			return err
		} else {
			bunVersion = v
		}
		log.InfoStatusEvent(os.Stdout, "Runtime is Bun (Version %s), %s", bunVersion, entryJSFile)

		cmd := exec.Command(bunPath, "run", entryJSFile)
		cmd.Dir = w.workDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = env

		return cmd.Run()
	}

	// if bun is not found, fallback to nodejs
	cmd := exec.Command(w.nodePath, entryJSFile)
	cmd.Dir = w.workDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env

	var nodeVersion string
	if v, err := checkVersion("node"); err != nil {
		return err
	} else {
		nodeVersion = v
	}

	log.InfoStatusEvent(os.Stdout, "Runtime is Node.js (Version %s), %s, %s", nodeVersion, w.nodePath, entryJSFile)

	return cmd.Run()
}

func (w *NodejsWrapper) genWrapperTS(dstPath string) error {
	baseFilename := "./" + filepath.Base(w.fileName)
	entryTS := "./dist/" + baseFilename + ".ts"

	data := struct {
		WorkDir  string
		FileName string
		FilePath string
	}{
		WorkDir:  "./",
		FileName: baseFilename,
		FilePath: entryTS,
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
		gitignoreContent := `# Node.js
node_modules/
npm-debug.log*
yarn-debug.log*
yarn-error.log*
.pnp/
.pnp.js

# Build artifacts
dist/
build/
out/
coverage/

# Editor/IDE specific
.vscode/
.idea/
*.sublime-project
*.sublime-workspace

# OS generated files
.DS_Store
Thumbs.db

# Environment variables
.env
.env.local
.env.development.local
.env.test.local
.env.production.local

# npm cache and logs
.npm/
npm-cache/
*.tgz

# Test results
junit.xml
test-results.xml

# YoMo specific
.wrapper.ts
`
		err = os.WriteFile(gitignore, []byte(gitignoreContent), 0644)
		if err != nil {
			return fmt.Errorf("write .gitignore failed: %v", err)
		}
	}

	return nil
}

func checkVersion(cmd string) (string, error) {
	versionCmd := exec.Command(cmd, "--version")
	versionOutput, versionErr := versionCmd.Output()
	if versionErr == nil {
		// need remove the trailing newline character
		cmdVersion := strings.TrimSpace(string(versionOutput))
		// log.InfoStatusEvent(os.Stdout, "%s is found :%s", cmd, cmdVersion)
		return cmdVersion, nil
	} else {
		log.InfoStatusEvent(os.Stdout, "%s is found, but failed to get version: %v", cmd, versionErr)
	}
	return "", versionErr
}
