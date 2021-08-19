package zipper

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
	fmt.Println("======== server Test Begin ========")
	code := m.Run()
	fmt.Println("========= server Test End =========")
	os.Exit(code)
}
