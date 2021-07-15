package source

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	fmt.Println("======== source Test Begin ========")
	code := m.Run()
	fmt.Println("========= source Test End =========")
	os.Exit(code)
}
