package dispatcher

import (
	"path/filepath"
	"plugin"

	"github.com/yomorun/yomo/internal/serverless"
	"github.com/yomorun/yomo/pkg/rx"
)

func Dispatcher(hanlder plugin.Symbol, rxstream rx.RxStream) rx.RxStream {
	return hanlder.(func(rxStream rx.RxStream) rx.RxStream)(rxstream)
}

func AutoDispatcher(appPath string, rxstream rx.RxStream) (rx.RxStream, error) {
	file := appPath
	// skip building if the extension is not .go
	// For example, already built as .so in the previous step.
	if filepath.Ext(appPath) == ".go" {
		sofile, err := serverless.Build(appPath, true)
		if err != nil {
			return nil, err
		}
		file = sofile
	}

	handler, err := serverless.LoadHandler(file)
	if err != nil {
		return nil, err
	}
	return Dispatcher(handler, rxstream), nil
}
