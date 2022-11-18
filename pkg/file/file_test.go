package file

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFile(t *testing.T) {
	dir := TempDir()
	testdir := filepath.Join(dir, "yomo", "test")

	srcpath := filepath.Join(testdir, "src")

	content := []byte("hello yomo")

	err := PutContents(srcpath, content)
	assert.NoError(t, err)

	gotContent, _ := os.ReadFile(srcpath)
	assert.Equal(t, content, gotContent)

	more := []byte("more")

	err = AppendContents(srcpath, more)
	assert.NoError(t, err)

	gotMoreContent, _ := os.ReadFile(srcpath)
	assert.Equal(t, append(gotContent, more...), gotMoreContent)

	dstpath := filepath.Join(testdir, "dst", "dst")

	err = Copy(srcpath, dstpath)
	assert.NoError(t, err)

	dstContent := GetContents(dstpath)
	assert.EqualValues(t, append(gotContent, more...), dstContent)

	err = Truncate(dstpath, 0)
	assert.NoError(t, err)
	dstContent = GetContents(dstpath)
	assert.Equal(t, "", dstContent)

	assert.True(t, IsExec("yomo.yomo"))
	assert.False(t, IsExec(dstpath))

	err = Remove(srcpath)
	assert.NoError(t, err)
	assert.False(t, Exists(srcpath))

	err = Remove(testdir)
	assert.NoError(t, err)
	assert.False(t, Exists(testdir))
}
