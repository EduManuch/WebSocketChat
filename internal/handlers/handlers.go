package handlers

import (
	"WebSocketChat/internal/types"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"net/http"
)

type WsHandler struct {
	Upgrader *websocket.Upgrader
}

func (h *WsHandler) CreateWsConnection(w http.ResponseWriter, r *http.Request, ws types.WsServer) {
	conn, err := h.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("Error with websocket connection: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	log.Debugf("Client with address %s connected", conn.RemoteAddr().String())

	client := ws.NewClient(conn)
	connChan := ws.GetConnChan()
	connChan <- client
	go ws.ReadFromClient(client)
	go ws.WriteToClient(client)
}
