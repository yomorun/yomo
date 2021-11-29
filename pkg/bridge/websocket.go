package bridge

import (
	"encoding/json"
	"net/http"
	"net/url"
	"sync"

	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/pkg/logger"
	"golang.org/x/net/websocket"
)

const defaultRoomID = "default"

// WebSocketBridge implements the Bridge interface for WebSocket.
type WebSocketBridge struct {
	addr   string
	server *websocket.Server

	// Registered the connections in each room.
	// Key: room id (string)
	// Value: conns in room (sync.Map)
	rooms sync.Map
}

// NewWebSocketBridge initializes an instance for WebSocketBridge.
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
		rooms: sync.Map{},
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
		// set payload type
		c.PayloadType = websocket.BinaryFrame
		// TODO: support multi rooms.
		roomID := defaultRoomID
		conns := ws.getConnsByRoomID(roomID)

		// register new connections.
		conns.Store(c, true)
		ws.rooms.Store(roomID, conns)

		// trigger the YoMo Server's Handler in bridge.
		handler(&core.Context{
			ConnID:       c.Request().RemoteAddr,
			Stream:       c,
			SendDataBack: ws.Send,
			OnClose: func(code uint64, msg string) {
				// remove this connection in room.
				conns := ws.getConnsByRoomID(roomID)
				conns.Delete(c)
				// broadcast offline message to clients
				ws.broadcastOffline(c)
			},
		})
	}

	// serve
	return http.ListenAndServe(ws.addr, ws.server)
}

// Send the data to WebSocket clients.
func (ws *WebSocketBridge) Send(f frame.Frame) error {
	// TODO: get RoomID from MetaFrame.
	roomID := defaultRoomID
	conns := ws.getConnsByRoomID(roomID)
	conns.Range(func(key, value interface{}) bool {
		if c, ok := key.(*websocket.Conn); ok {
			_, err := c.Write(f.Encode())
			if err != nil {
				logger.Errorf("[WebSocketBridge] send data to conn failed, roomID=%s", roomID)
			}
		}
		return true
	})
	return nil
}

func (ws *WebSocketBridge) getConnsByRoomID(roomID string) sync.Map {
	v, ok := ws.rooms.Load(roomID)
	if !ok || v == nil {
		v = sync.Map{}
	}
	conns, _ := v.(sync.Map)
	return conns
}

// TODO: we need to find a good solution to broadcast the `offline` message.
func (ws *WebSocketBridge) broadcastOffline(c *websocket.Conn) error {
	clientID := c.Request().URL.Query().Get("clientId")
	if clientID == "" {
		clientID = c.Request().RemoteAddr
	}
	payload := presence{
		Event: "offline",
		Data: presencePayload{
			ID: clientID,
		},
	}
	carrige, _ := json.Marshal(payload)
	dataFrame := frame.NewDataFrame()
	dataFrame.SetCarriage(0x11, carrige)
	return ws.Send(dataFrame)
}

// presence is the predefined data structure for presence. (f.e. offline event)
type presence struct {
	Event string          `json:"event"`
	Data  presencePayload `json:"data"`
}

type presencePayload struct {
	ID string `json:"id"`
}
