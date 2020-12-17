package serverless

import (
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
)

func Build(appPath string) (string, error) {
	version := runtime.GOARCH
	dir, _ := filepath.Split(appPath)

	if version == "amd64" {
		cmd := exec.Command("/bin/sh", "-c", "go build -buildmode=plugin -o "+dir+"sl.so "+appPath)
		_, err := cmd.Output()

		if err != nil {
			return "", err
		}
		return dir + "sl.so", nil
	} else {
		return "", errors.New("Not Implemented")
	}

}
