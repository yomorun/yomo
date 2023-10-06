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

// MainFuncTmpl the raw bytes serverless of the main function template
//
//go:embed templates/main.tmpl
var MainFuncTmpl []byte

// PartialsTmpl partials template, used for rendering the partials
//
//go:embed templates/partials.tmpl
var PartialsTmpl []byte

//go:embed templates/init.tmpl
var InitTmpl []byte

//go:embed templates/init_rx.tmpl
var InitRxTmpl []byte

//go:embed templates/wasm_main.tmpl
var WasmMainFuncTmpl []byte

// Context defines context for the template
type Context struct {
	// Name of the servcie
	Name string
	// ZipperAddrs is the address of the zipper server
	ZipperAddr string
	// Client credential
	Credential string
	// use environment variables
	UseEnv bool
	// WithInitFunc determines whether to work with init function
	WithInitFunc bool
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
