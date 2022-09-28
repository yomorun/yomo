package file

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Copy file from src to dst
func Copy(src string, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	dir := Dir(dst)
	if !Exists(dir) {
		err := Mkdir(dir)
		if err != nil {
			return err
		}
	}
	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return err
	}
	err = dstFile.Sync()
	if err != nil {
		return err
	}
	srcFile.Close()
	dstFile.Close()
	return nil
}

// Dir return file dir
func Dir(filename string) string {
	return filepath.Dir(filename)
}

// Mkdir create dir like mkdir -p
func Mkdir(fpath string) error {
	err := os.MkdirAll(fpath, os.ModePerm) // generate all dir
	return err
}

// Exists check is file exists
func Exists(path string) bool {
	_, err := os.Stat(path)
	if err != nil {
		return os.IsExist(err)
	}
	return true
}

func putContents(path string, data []byte, flag int, perm os.FileMode) error {
	// create dir recursively
	dir := Dir(path)
	if !Exists(dir) {
		if err := Mkdir(dir); err != nil {
			return err
		}
	}
	// create and open file
	f, err := os.OpenFile(path, flag, perm)
	if err != nil {
		return err
	}
	defer f.Close()
	n, err := f.Write(data)
	if err != nil {
		return err
	} else if n < len(data) {
		return io.ErrShortWrite
	}
	return nil
}

// Truncate changes the size of the named file.
func Truncate(path string, size int) error {
	return os.Truncate(path, int64(size))
}

// PutContents write content to given file
func PutContents(path string, content []byte) error {
	return putContents(path, []byte(content), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
}

// AppendContents append content to give file
func AppendContents(path string, content []byte) error {
	return putContents(path, content, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
}

// TempDir returns the OS temp dir
func TempDir() string {
	return os.TempDir()
}

// Remove file
func Remove(path string) error {
	return os.RemoveAll(path)
}

// GetContents read file content
func GetContents(path string) string {
	return string(GetBinContents(path))
}

// GetBinContents read file content as bytes
func GetBinContents(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return data
}

func IsExec(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == ".yomo" || ext == ".exe" {
		return true
	}
	return false
}
