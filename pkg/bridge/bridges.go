package bridge

import (
	"fmt"

	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/pkg/config"
	"github.com/yomorun/yomo/pkg/logger"
)

const (
	nameOfWebSocket = "websocket"
)

// InitBridges initialize the bridges from conf.
func InitBridges(conf *config.WorkflowConfig) []core.Bridge {
	bridges := make([]core.Bridge, 0)
	if conf.Bridges == nil {
		return bridges
	}

	for _, cb := range conf.Bridges {
		// all bridges will be running in the same host of zipper.
		addr := fmt.Sprintf("%s:%d", conf.Host, cb.Port)

		switch cb.Name {
		case nameOfWebSocket:
			bridges = append(bridges, NewWebSocketBridge(addr))
		default:
			logger.Errorf("InitBridges: the name of bridge %s is not implemented", cb.Name)
		}
	}

	return bridges
}
