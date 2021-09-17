package rx

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	fmt.Println("======== core/rx Test Begin ========")
	code := m.Run()
	fmt.Println("========= core/rx Test End =========")
	os.Exit(code)
}
