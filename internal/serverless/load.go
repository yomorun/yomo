package serverless

import (
	"fmt"
	"plugin"
)

func LoadHandle(filePath string) (plugin.Symbol, error) {
	plugin, err := plugin.Open(filePath)
	if err != nil {
		fmt.Println("open plugin error", err)
		return nil, err
	}

	hanlder, err := plugin.Lookup("Hanlder")
	if err != nil {
		fmt.Println("lookup plugin error", err)
		return nil, err
	}

	return hanlder, nil
}
