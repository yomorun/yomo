// Package yomo test main.s
package yomo

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	zipper := NewZipperWithOptions("test-zipper", "localhost:9000")
	defer zipper.Close()
	zipper.ConfigWorkflow("test/workflow.yaml")
	go zipper.ListenAndServe()

	code := m.Run()
	os.Exit(code)
}
