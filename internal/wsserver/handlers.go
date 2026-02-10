package wsserver

import (
	log "github.com/sirupsen/logrus"
	"net/http"
)

func (ws *wsSrv) wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := ws.wsUpg.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("Error with websocket connection: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	log.Debugf("Client with address %s connected", conn.RemoteAddr().String())

	client := newClient(conn)
	ws.connChan <- client
	go ws.readFromClient(client)
	go ws.writeToClient(client)
}
