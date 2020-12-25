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
actions:
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

	if wf.Sources[0].Name != "Emitter server" {
		t.Errorf("Sources[0].Name value should be `%v`", "Emitter server")
	}

	if wf.Actions[0].Name != "Noise Serverless" {
		t.Errorf("Actions[0].Name value should be `%v`", "Noise Serverless")
	}

	if wf.Sinks[0].Name != "Mock DB" {
		t.Errorf("Sinks[0].Name value should be `%v`", "Mock DB")
	}
}
