package serverless

import _ "unsafe"

//go:linkname Description main.Description
func Description() string

//go:linkname InputSchema main.InputSchema
func InputSchema() any
