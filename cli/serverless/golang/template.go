package golang

import (
	"bytes"
	_ "embed"
	"text/template"
)

// MainFuncRxTmpl the rxstream serverless of the main function template
//
//go:embed templates/main_rx.tmpl
var MainFuncRxTmpl []byte

// MainFuncRawBytesTmpl the raw bytes serverless of the main function template
//
//go:embed templates/main_raw_bytes.tmpl
var MainFuncRawBytesTmpl []byte

// PartialsTmpl partials template, used for rendering the partials
//
//go:embed templates/partials.tmpl
var PartialsTmpl []byte

//go:embed templates/init.tmpl
var InitFuncTmpl []byte

//go:embed templates/init_raw.tmpl
var InitRawFuncTmpl []byte

//go:embed templates/wasm_main_raw.tmpl
var WasmMainFuncRawTmpl []byte

// Context defines context for the template
type Context struct {
	// Name of the servcie
	Name string
	// ZipperAddrs is the addresses of the zipper server
	ZipperAddrs []string
	// Client credential
	Credential string
	// use environment variables
	UseEnv bool
}

// RenderTmpl renders the template with the given context
func RenderTmpl(tpl string, ctx *Context) ([]byte, error) {
	t := template.Must(template.New("tpl").Parse(tpl))
	buf := bytes.NewBuffer([]byte{})
	err := t.Execute(buf, ctx)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
