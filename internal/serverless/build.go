package serverless

import (
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
)

func Build(appPath string) (string, error) {
	version := runtime.GOOS
	dir, _ := filepath.Split(appPath)
	sl := dir + "sl.so"

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
