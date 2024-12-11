// Package yomo test main.s
package yomo

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	// setup zipper
	go RunZipper(context.TODO(), "test/config.yaml")
	time.Sleep(time.Second)

	code := m.Run()
	os.Exit(code)
}
