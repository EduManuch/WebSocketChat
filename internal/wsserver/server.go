package wsserver

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"
)

type WSServer interface {
	Start(cert, key, templateDir, staticDir string, useKafka bool) error
	Stop() error
}

type wsSrv struct {
	mux         *http.ServeMux
	srv         *http.Server
	wsUpg       *websocket.Upgrader
	broadcast   chan *WsMessage
	clients     clients
	connChan    chan *websocket.Conn
	delConnChan chan *websocket.Conn
	wsKafka     Kafka
	host        string
}

type clients struct {
	mutex     *sync.RWMutex
	wsClients map[*websocket.Conn]struct{}
}

type Kafka struct {
	Producer *kafka.Producer
	Consumer *kafka.Consumer
}

func NewWsServer(addr string, useKafka bool) WSServer {
	m := http.NewServeMux()
	hostname, _ := os.Hostname()
	var k Kafka
	if useKafka {
		k = Kafka{
			Producer: NewProducer("kafka:9092"),
			Consumer: NewConsumer("kafka:9092", hostname),
		}
	}

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
				switch addrUrl.Host { // тут разрешаем соединение из списка хостов
				case addr, "127.0.0.1:8443", "127.0.0.1:8444", "localhost:8443", "localhost:8444":
					return true
				default:
					return false
				}
			},
		},
		broadcast: make(chan *WsMessage),
		clients: clients{
			mutex:     &sync.RWMutex{},
			wsClients: map[*websocket.Conn]struct{}{},
		},
		connChan: make(chan *websocket.Conn, 1),
		wsKafka:  k,
		host:     hostname,
	}
}

func (ws *wsSrv) Start(cert, key, templateDir, staticDir string, useKafka bool) error {
	certPair, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		return fmt.Errorf("failed certificate pair: %w", err)
	}
	ws.srv.TLSConfig.Certificates = append(ws.srv.TLSConfig.Certificates, certPair)
	ws.mux.Handle("/", http.FileServer(http.Dir(templateDir)))
	ws.mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))
	ws.mux.HandleFunc("/ws", ws.wsHandler)
	//go ws.controlClientsConn()
	go ws.addClientConn()
	go ws.delClientConn()
	go ws.safeWrite(useKafka)
	if useKafka {
		go ws.GetProducerEventsKafka()
		go ws.ReceiveKafka()
	}
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
		// удаление соединений при остановке сервера
		//delete(ws.clients.wsClients, conn)
		//ws.connChan <- conn
		ws.delConnChan <- conn
	}
	ws.clients.mutex.Unlock()
	log.Info("Clients list after close", ws.clients.wsClients)
	ws.wsKafka.Producer.Flush(15 * 1000)
	ws.wsKafka.Consumer.Close()
	ws.wsKafka.Producer.Close()
	log.Info("Kafka producer Flush done. Producer and consumer closed")
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

	ws.connChan <- conn
	go ws.safeRead(conn)
}

//func (ws *wsSrv) controlClientsConn() {
//	for conn := range ws.connChan {
//		ws.clients.mutex.Lock()
//		// Если соединения нет в мапе, значит было создано новое
//		if _, ok := ws.clients.wsClients[conn]; !ok {
//			fmt.Println("СОЗДАЛИ")
//			ws.clients.wsClients[conn] = struct{}{}
//		} else { // иначе соединение отправлено для удаления
//			fmt.Println("УДАЛИЛИ")
//			delete(ws.clients.wsClients, conn)
//		}
//		ws.clients.mutex.Unlock()
//	}
//}

func (ws *wsSrv) addClientConn() {
	for conn := range ws.connChan {
		ws.clients.mutex.Lock()
		// Если соединения нет в мапе, значит было создано новое
		if _, ok := ws.clients.wsClients[conn]; !ok {
			log.Println("Создано новое соединение")
			ws.clients.wsClients[conn] = struct{}{}
		}
		ws.clients.mutex.Unlock()
	}
}

func (ws *wsSrv) delClientConn() {
	for conn := range ws.delConnChan {
		ws.clients.mutex.Lock()
		log.Println("Удалено соединение")
		delete(ws.clients.wsClients, conn)
		ws.clients.mutex.Unlock()
	}
}

func (ws *wsSrv) safeRead(conn *websocket.Conn) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("error launching write worker: %v", r)
		}
	}()

	ch := make(chan int, 1)
	goFunc := func() {
		ws.readFromClient(conn, ch)
	}

	ch <- 1
	for _ = range ch {
		go goFunc()
	}
}

func (ws *wsSrv) readFromClient(conn *websocket.Conn, c chan<- int) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("error in read worker : %v", r)
			c <- 1
		}
	}()

	for {
		msg := new(WsMessage)
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
	// удаление соединения из мапы
	//ws.connChan <- conn
	ws.delConnChan <- conn
}

func (ws *wsSrv) safeWrite(useKafka bool) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("error launching write worker: %v", r)
		}
	}()

	ch := make(chan int, 1)
	goFunc := func() {
		ws.writeToClientsBroadCast(ch, useKafka)
	}

	ch <- 1
	for _ = range ch {
		go goFunc()
	}
}

func (ws *wsSrv) writeToClientsBroadCast(c chan<- int, useKafka bool) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("error in write worker : %v", r)
			ws.clients.mutex.Unlock()
			c <- 1
		}
	}()

	for msg := range ws.broadcast {
		for client := range ws.clients.wsClients {
			if err := client.WriteJSON(msg); err != nil {
				log.Errorf("Error with writing message: %v", err)
				ws.clients.mutex.Lock()
				delete(ws.clients.wsClients, client)
				ws.clients.mutex.Unlock()
			} else if useKafka {
				ws.SendKafka(msg)
			}
		}
	}
}
