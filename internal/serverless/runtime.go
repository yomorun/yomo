package serverless

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"plugin"
)

// Run runs the serverless.so file.
func Run(file string, endpoint string) error {
	p, err := plugin.Open(file)
	if err != nil {
		return err
	}

	// validate if the plugin contains "Handle" function
	_, err = p.Lookup("Handle")
	if err != nil {
		return err
	}

	// TODO: host the endpoint
	log.Printf("Mock hosting the endpoint: %s", endpoint)

	return nil
}

// BuildFuncFile builds the Serverless Function as a .so file.
// Returns the path of serverless.so
func BuildFuncFile(file string) (string, error) {
	// check if the file exists
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return "", err
	}

	// get the path for serverless.so
	dir, _ := filepath.Split(file)
	if dir == "" {
		dir = "./"
	}
	sl := dir + "serverless.so"

	// build the file as serverless.so
	cmd := exec.Command("go", "build", "-buildmode", "plugin", "-o", sl, file)
	err := cmd.Start()
	if err != nil {
		return "", err
	}
	err = cmd.Wait()
	return sl, err
}
