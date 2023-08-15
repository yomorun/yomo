// Package id provides id generation
package id

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"time"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// New generate random id.
func New(l ...int) string {
	tid, err := gonanoid.New(l...)
	if err != nil {
		tid = strconv.FormatInt(time.Now().UnixMicro(), 10)
	}
	return tid
}

// TID generate trace id.
func TID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return ""
	}
	return hex.EncodeToString(bytes)
}

// SID generate span id.
func SID() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return ""
	}
	return hex.EncodeToString(bytes)
}
