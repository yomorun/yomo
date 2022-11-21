package yomo

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	zipper := NewZipperWithOptions("test-zipper")
	defer zipper.Close()
	zipper.ConfigWorkflow("test/workflow.yaml")
	go zipper.ListenAndServe()

	code := m.Run()
	os.Exit(code)
}
