package golang

import (
	"bytes"
	_ "embed"
	"text/template"
)

//go:embed templates/main.tmpl
var MainFuncTmpl []byte

//go:embed templates/init.tmpl
var InitFuncTmpl []byte

// Context main function data
type Context struct {
	Name string
	Host string
	Port int
}

func RenderTmpl(tpl string, ctx *Context) ([]byte, error) {
	t := template.Must(template.New("tpl").Parse(tpl))
	buf := bytes.NewBuffer([]byte{})
	err := t.Execute(buf, ctx)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
