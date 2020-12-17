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
	sl := dir + "sl.so"

	if version == "amd64" {
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
