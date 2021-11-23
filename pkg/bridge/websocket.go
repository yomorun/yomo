package bridge

import (
	"net/http"
	"net/url"

	"github.com/yomorun/yomo/core"
	"golang.org/x/net/websocket"
)

// WebSocketBridge implements the Bridge interface for WebSocket.
type WebSocketBridge struct {
	addr   string
	server *websocket.Server
}

// NewWebSocketBridge initializes a instance for WebSocketBridge.
func NewWebSocketBridge(addr string) *WebSocketBridge {
	return &WebSocketBridge{
		addr: addr,
		server: &websocket.Server{
			Config: websocket.Config{
				Origin: &url.URL{
					Host: addr,
				},
			},
			Handshake: func(c *websocket.Config, r *http.Request) error {
				// TODO: check Origin header for auth.
				return nil
			},
		},
	}
}

// Name returns the name of WebSocket bridge.
func (ws *WebSocketBridge) Name() string {
	return nameOfWebSocket
}

// Addr returns the address of bridge.
func (ws *WebSocketBridge) Addr() string {
	return ws.addr
}

// ListenAndServe starts a WebSocket server with a given handler.
func (ws *WebSocketBridge) ListenAndServe(handler func(ctx *core.Context)) error {
	// wrap the WebSocket handler.
	ws.server.Handler = func(c *websocket.Conn) {
		// trigger the YoMo Server's Handler in bridge.
		handler(&core.Context{
			Stream:       c,
			SendDataBack: true,
		})
	}

	// serve
	return http.ListenAndServe(ws.addr, ws.server)
}
