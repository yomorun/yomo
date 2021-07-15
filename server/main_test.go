package server

import (
	"fmt"
	"os"
	"testing"

	"github.com/yomorun/yomo/logger"
)

var (
	testConfig  *WorkflowConfig
	testMeshURL string
)

func TestMain(m *testing.M) {
	logger.EnableDebug()
	var err error
	testConfig, err = ParseConfig("./mock/workflow.yaml")
	if err != nil {
		panic(err)
	}
	fmt.Println("======== Server Test Begin ========")
	code := m.Run()
	fmt.Println("========= Server Test End =========")
	os.Exit(code)
}
