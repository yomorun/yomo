package yomo

import (
	"os"
	"testing"
)

var (
	testConfig  *WorkflowConfig
	testMeshURL string
)

func TestMain(m *testing.M) {
	zipper := NewZipperServer("zipper", WithZipperListenAddr("localhost:9000"))
	defer zipper.Close()
	zipper.ConfigWorkflow("test/workflow.yaml")
	go zipper.ListenAndServe()

	code := m.Run()
	os.Exit(code)
}
