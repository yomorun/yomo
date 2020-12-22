package serverless

import (
	"fmt"
	"plugin"
)

func LoadHandler(filePath string) (plugin.Symbol, error) {
	plugin, err := plugin.Open(filePath)
	if err != nil {
		fmt.Println("open plugin error", err)
		return nil, err
	}

	handler, err := plugin.Lookup("Handler")
	if err != nil {
		fmt.Println("lookup plugin error", err)
		return nil, err
	}

	return handler, nil
}
