// Package id provides id generation
package id

import (
	"strconv"
	"time"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// New generate id.
func New() string {
	tid, err := gonanoid.New()
	if err != nil {
		tid = strconv.FormatInt(time.Now().UnixMicro(), 10)
	}
	return tid
}
