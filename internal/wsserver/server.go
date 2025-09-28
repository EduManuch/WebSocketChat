package wsserver

import (
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	templateDir = "./web/templates/html"
	staticDir   = "./web/static"
)

type WSServer interface {
	Start() error
}

type wsSrv struct {
	mux       *http.ServeMux
	srv       *http.Server
	wsUpg     *websocket.Upgrader
	wsClients map[*websocket.Conn]struct{}
	mutex     *sync.RWMutex
	broadcast chan *wsMessage
}

func NewWsServer(addr string) WSServer {
	m := http.NewServeMux()
	return &wsSrv{
		mux: m,
		srv: &http.Server{
			Addr:    addr,
			Handler: m,
		},
		wsUpg:     &websocket.Upgrader{},
		wsClients: map[*websocket.Conn]struct{}{},
		mutex:     &sync.RWMutex{},
		broadcast: make(chan *wsMessage),
	}
}

func (ws *wsSrv) Start() error {
	ws.mux.Handle("/", http.FileServer(http.Dir(templateDir)))
	ws.mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))
	ws.mux.HandleFunc("/ws", ws.wsHandler)
	ws.mux.HandleFunc("/test", ws.testHandler)
	go ws.writeToClientsBroadCast()
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
	log.Infof("Client with address %s connected", conn.RemoteAddr().String())
	ws.mutex.Lock()
	ws.wsClients[conn] = struct{}{}
	ws.mutex.Unlock()
	go ws.readFromClient(conn)
}

func (ws *wsSrv) readFromClient(conn *websocket.Conn) {
	for {
		msg := new(wsMessage)
		err := conn.ReadJSON(msg)
		if err != nil {
			log.Errorf("Error with reading from Websocket: %v", err)
			break
		}
		host, _, err := net.SplitHostPort(conn.RemoteAddr().String())
		if err != nil {
			log.Errorf("Error with address split: %v", err)
		}
		msg.IPAddress = host
		msg.Time = time.Now().Format("15:04")
		ws.broadcast <- msg
	}
	ws.mutex.Lock()
	delete(ws.wsClients, conn)
	ws.mutex.Unlock()
}

func (ws *wsSrv) writeToClientsBroadCast() {
	for msg := range ws.broadcast {
		ws.mutex.RLock()
		for client := range ws.wsClients {
			go func() {
				if err := client.WriteJSON(msg); err != nil {
					log.Errorf("Error with writing message: %v", err)
				}
			}()
		}
		ws.mutex.RUnlock()
	}
}
