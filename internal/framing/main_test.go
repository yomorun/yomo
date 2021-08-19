package framing

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	fmt.Println("======== internal/framing Test Begin ========")
	code := m.Run()
	fmt.Println("========= internal/framing Test End =========")
	os.Exit(code)
}
