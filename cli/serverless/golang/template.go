package golang

import (
	"bytes"
	_ "embed"
	"text/template"
)

//go:embed templates/main.tmpl
var MainFuncTmpl []byte

// Context defines context for the template
type Context struct {
	// Name of the servcie
	Name string
	// ZipperAddr is the address of the zipper server
	ZipperAddr string
	// Client credential
	Credential string
	// WithInitFunc determines whether to work with init function
	WithInitFunc bool
	// WithWantedTarget determines whether to work with SetWantedTarget
	WithWantedTarget bool
	// WithDescription determines whether to work with description
	WithDescription bool
	// WithInputSchema determines whether to work with input schema
	WithInputSchema bool
	// WithDataTags determines whether to work with data tags
	WithDataTags bool
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
