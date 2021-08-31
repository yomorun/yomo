package file

import (
	"io"
	"os"
	"path/filepath"
)

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

func Dir(filename string) string {
	return filepath.Dir(filename)
}

func Mkdir(fpath string) error {
	err := os.MkdirAll(fpath, os.ModePerm) // 生成多级目录
	return err
}

func Exists(path string) bool {
	_, err := os.Stat(path)
	if err != nil {
		return os.IsExist(err)
	}
	return true
}

func putContents(path string, data []byte, flag int, perm os.FileMode) error {
	// 支持目录递归创建
	dir := Dir(path)
	if !Exists(dir) {
		if err := Mkdir(dir); err != nil {
			return err
		}
	}
	// 创建/打开文件
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

// PutContents (文本)写入文件内容
func PutContents(path string, content []byte) error {
	return putContents(path, []byte(content), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
}

// AppendContents (文本)追加内容到文件末尾
func AppendContents(path string, content []byte) error {
	return putContents(path, content, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
}

func TempDir() string {
	return os.TempDir()
}

func Remove(path string) error {
	return os.RemoveAll(path)
}

func GetContents(path string) string {
	return string(GetBinContents(path))
}

// GetBinContents (二进制)读取文件内容
func GetBinContents(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return data
}
