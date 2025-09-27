package wsserver

import (
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"net/http"
)

type WSServer interface {
	Start() error
}

type wsSrv struct {
	mux   *http.ServeMux
	srv   *http.Server
	wsUpg *websocket.Upgrader
}

func NewWsServer(addr string) WSServer {
	m := http.NewServeMux()
	return &wsSrv{
		mux: m,
		srv: &http.Server{
			Addr:    addr,
			Handler: m,
		},
		wsUpg: &websocket.Upgrader{},
	}
}

func (ws *wsSrv) Start() error {
	ws.mux.HandleFunc("/ws", ws.wsHandler)
	ws.mux.HandleFunc("/test", ws.testHandler)
	return ws.srv.ListenAndServe()
}

func (ws *wsSrv) testHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Test is successful"))
}

func (ws *wsSrv) wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := ws.wsUpg.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("Error with websocket connection: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	log.Infof(conn.RemoteAddr().String())
}
