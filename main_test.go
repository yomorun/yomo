// Package yomo test main.s
package yomo

import (
	"context"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	go RunZipper(context.TODO(), "test/config.yaml")
	code := m.Run()
	os.Exit(code)
}
