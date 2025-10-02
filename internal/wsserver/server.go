package wsserver

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type WSServer interface {
	Start(cert, key, templateDir, staticDir string) error
	Stop() error
}

type wsSrv struct {
	mux       *http.ServeMux
	srv       *http.Server
	wsUpg     *websocket.Upgrader
	broadcast chan *wsMessage
	clients   clients
}

type clients struct {
	mutex     *sync.RWMutex
	wsClients map[*websocket.Conn]struct{}
}

func NewWsServer(addr string) WSServer {
	m := http.NewServeMux()
	return &wsSrv{
		mux: m,
		srv: &http.Server{
			Addr:    addr,
			Handler: m,
			TLSConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
				MaxVersion: tls.VersionTLS12,
				CipherSuites: []uint16{
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				},
			},
		},
		wsUpg: &websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				addrUrl, err := url.ParseRequestURI(origin)
				if err != nil {
					return false
				}
				return addrUrl.Host == addr // тут разрешил соединение только со своего адреса
			},
		},
		broadcast: make(chan *wsMessage),
		clients: clients{
			mutex:     &sync.RWMutex{},
			wsClients: map[*websocket.Conn]struct{}{},
		},
	}
}

func (ws *wsSrv) Start(cert, key, templateDir, staticDir string) error {
	certPair, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		return fmt.Errorf("failed certificate pair: %w", err)
	}
	ws.srv.TLSConfig.Certificates = append(ws.srv.TLSConfig.Certificates, certPair)
	ws.mux.Handle("/", http.FileServer(http.Dir(templateDir)))
	ws.mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))
	ws.mux.HandleFunc("/ws", ws.wsHandler)
	go ws.writeToClientsBroadCast()
	//return ws.srv.ListenAndServeTLS(cert, key)
	return ws.srv.ListenAndServeTLS("", "")
}

func (ws *wsSrv) Stop() error {
	log.Info("Before close", ws.clients.wsClients)
	close(ws.broadcast)
	ws.clients.mutex.Lock()
	for conn := range ws.clients.wsClients {
		if err := conn.Close(); err != nil {
			log.Errorf("Error with closing: %v", err)
		}
		delete(ws.clients.wsClients, conn)
	}
	ws.clients.mutex.Unlock()
	log.Info("After close", ws.clients.wsClients)
	return ws.srv.Shutdown(context.Background())
}

func (ws *wsSrv) wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := ws.wsUpg.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("Error with websocket connection: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	log.Infof("Client with address %s connected", conn.RemoteAddr().String())
	ws.clients.mutex.Lock()
	ws.clients.wsClients[conn] = struct{}{}
	ws.clients.mutex.Unlock()
	go ws.readFromClient(conn)
}

func (ws *wsSrv) readFromClient(conn *websocket.Conn) {
	for {
		msg := new(wsMessage)
		if err := conn.ReadJSON(msg); err != nil {
			var wsErr *websocket.CloseError
			ok := errors.As(err, &wsErr)
			if !ok || wsErr.Code != websocket.CloseGoingAway {
				log.Errorf("Error with reading from Websocket: %v", err)
			}
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
	ws.clients.mutex.Lock()
	delete(ws.clients.wsClients, conn)
	ws.clients.mutex.Unlock()
}

func (ws *wsSrv) writeToClientsBroadCast() {
	for msg := range ws.broadcast {
		ws.clients.mutex.RLock()
		for client := range ws.clients.wsClients {
			go func() {
				if err := client.WriteJSON(msg); err != nil {
					log.Errorf("Error with writing message: %v", err)
				}
			}()
		}
		ws.clients.mutex.RUnlock()
	}
}
