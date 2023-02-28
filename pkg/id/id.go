// Package id provides id generation
package id

import (
	gonanoid "github.com/matoous/go-nanoid/v2"
)

// New generate id
func New() string { return gonanoid.Must() }
