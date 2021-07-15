package output

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	fmt.Println("======== connector/output Test Begin ========")
	code := m.Run()
	fmt.Println("========= connector/output Test End =========")
	os.Exit(code)
}
