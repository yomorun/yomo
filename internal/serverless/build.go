package serverless

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Build builds the serverless function to .so file.
func Build(appPath string, clean bool) (string, error) {
	// check if the file exists
	if _, err := os.Stat(appPath); os.IsNotExist(err) {
		return "", fmt.Errorf("the file %s doesn't exist", appPath)
	}

	// build
	version := runtime.GOOS
	dir, _ := filepath.Split(appPath)
	sl := dir + "sl.so"

	// clean build
	if clean {
		// .so file exists, remove it.
		if _, err := os.Stat(sl); !os.IsNotExist(err) {
			err = os.Remove(sl)
			if err != nil {
				return "", fmt.Errorf("clean build the file %s failed", appPath)
			}
		}
	}

	if version == "linux" {
		cmd := exec.Command("/bin/sh", "-c", "CGO_ENABLED=1 GOOS=linux go build -buildmode=plugin -o "+sl+" "+appPath)
		err := cmd.Start()
		if err != nil {
			return "", err
		}
		err = cmd.Wait()
		return sl, err
	} else if version == "darwin" {
		cmd := exec.Command("/bin/sh", "-c", "go build -buildmode=plugin -o "+sl+" "+appPath)
		err := cmd.Start()
		if err != nil {
			return "", err
		}
		err = cmd.Wait()
		return sl, err
	} else {
		return "", errors.New("Not Implemented")
	}

}
