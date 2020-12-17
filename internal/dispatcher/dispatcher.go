package dispatcher

import (
	"plugin"

	"github.com/yomorun/yomo/internal/serverless"
	"github.com/yomorun/yomo/pkg/rx"
)

func Dispatcher(hanlder plugin.Symbol, rxstream rx.RxStream) rx.RxStream {
	return hanlder.(func(rxStream rx.RxStream) rx.RxStream)(rxstream)
}

func AutoDispatcher(appPath string, rxstream rx.RxStream) (rx.RxStream, error) {
	sofile, err := serverless.Build(appPath)
	if err != nil {
		return nil, err
	}

	hanlder, err := serverless.LoadHandle(sofile)
	if err != nil {
		return nil, err
	}
	return Dispatcher(hanlder, rxstream), nil
}
