package decoder

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	fmt.Println("======== internal/decoder Test Begin ========")
	code := m.Run()
	fmt.Println("========= internal/decoder Test End =========")
	os.Exit(code)
}
