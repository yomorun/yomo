package streamfunction

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	fmt.Println("======== streamfunction Test Begin ========")
	code := m.Run()
	fmt.Println("========= streamfunction Test End =========")
	os.Exit(code)
}
