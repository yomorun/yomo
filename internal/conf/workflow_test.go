package conf

import (
	"testing"
)

func TestLoadData(t *testing.T) {
	var data = `
name: Service
host: localhost
port: 9999
sources:
  - name: Emitter server
    host: emitter.cella.fun
    port: 11521
flows:
  - name: Noise Serverless
    host: localhost
    port: 4242
sinks:
  - name: Mock DB
    host: localhost
    port: 4141
`
	wf, err := load([]byte(data))
	if err != nil {
		t.Errorf("%v", err)
	}

	if wf.Name != "Service" {
		t.Errorf("name value should be `%v`", "Service")
	}

	if wf.Host != "localhost" {
		t.Errorf("host value should be `%v`", "localhost")
	}

	if wf.Port != 9999 {
		t.Errorf("port value should be `%v`", "9999")
	}
}
