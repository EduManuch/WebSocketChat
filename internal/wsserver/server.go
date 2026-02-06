package wsserver

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"
)

type WSServer interface {
	Start(e *EnvConfig) error
	Stop(useKafka bool) error
}
type EnvConfig struct {
	Addr        string
	Port        string
	UseTls      bool
	CertFile    string
	KeyFile     string
	TemplateDir string
	StaticDir   string
	UseKafka    bool
	Origins     map[string]struct{}
	Debug       bool
}

type wsSrv struct {
	mux         *http.ServeMux
	srv         *http.Server
	wsUpg       *websocket.Upgrader
	broadcast   chan *WsMessage
	clients     clients
	connChan    chan *sClient //*websocket.Conn
	delConnChan chan *sClient //*websocket.Conn
	wsKafka     Kafka
	host        string
}

type clients struct {
	mutex     *sync.RWMutex
	wsClients map[*sClient]struct{} //map[*websocket.Conn]struct{}
}

type sClient struct {
	conn   *websocket.Conn
	once   sync.Once
	ctx    context.Context
	cancel context.CancelFunc
}

func (c *sClient) Close() {
	c.once.Do(func() {
		c.cancel()
		if err := c.conn.Close(); err != nil {
			log.Errorf("Error with closing: %v", err)
		}
	})
}

const (
	pongWait   = 10 * time.Second
	pingPeriod = 8 * time.Second
	writeWait  = 5 * time.Second
)

var (
	wsActiveConnections = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ws_active_connections",
			Help: " Websocket active connections",
		},
	)
)

type Kafka struct {
	Producer *kafka.Producer
	Consumer *kafka.Consumer
}

func NewWsServer(e *EnvConfig) WSServer {
	m := http.NewServeMux()

	prometheus.MustRegister(wsActiveConnections)
	wsActiveConnections.Set(0)

	if e.Debug {
		log.SetLevel(log.DebugLevel)
	}

	hostname, _ := os.Hostname()
	var k Kafka
	if e.UseKafka {
		k = Kafka{
			Producer: NewProducer("kafka:9092"),
			Consumer: NewConsumer("kafka:9092", hostname),
		}
	}

	addr := e.Addr + e.Port
	return &wsSrv{
		mux: m,
		srv: &http.Server{
			Addr:    addr,
			Handler: m,
		},
		wsUpg: &websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				addrUrl, err := url.ParseRequestURI(origin)
				if err != nil {
					return false
				}
				_, ok := e.Origins[addrUrl.Host]
				return ok
			},
		},
		broadcast: make(chan *WsMessage, 1024),
		clients: clients{
			mutex:     &sync.RWMutex{},
			wsClients: make(map[*sClient]struct{}, 1024),
		},
		connChan:    make(chan *sClient, 1024),
		delConnChan: make(chan *sClient, 1024),
		wsKafka:     k,
		host:        hostname,
	}
}

func newClient(conn *websocket.Conn) *sClient {
	ctx, cancel := context.WithCancel(context.Background())
	return &sClient{
		conn:   conn,
		ctx:    ctx,
		cancel: cancel,
	}
}

func (ws *wsSrv) Start(e *EnvConfig) error {
	ws.mux.Handle("/", http.FileServer(http.Dir(e.TemplateDir)))
	ws.mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(e.StaticDir))))
	ws.mux.HandleFunc("/ws", ws.wsHandler)
	ws.mux.Handle("/metrics", promhttp.Handler())
	go ws.addClientConn()
	go ws.delClientConn()
	go ws.writeToClientsBroadCast(e.UseKafka)
	if e.UseKafka {
		go ws.GetProducerEventsKafka()
		go ws.ReceiveKafka()
	}

	if e.UseTls {
		certPair, err := tls.LoadX509KeyPair(e.CertFile, e.KeyFile)
		if err != nil {
			return fmt.Errorf("failed certificate pair: %w", err)
		}
		ws.srv.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
			MaxVersion: tls.VersionTLS12,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			},
			Certificates: []tls.Certificate{certPair},
		}
		return ws.srv.ListenAndServeTLS("", "")
	}
	return ws.srv.ListenAndServe()
}

func (ws *wsSrv) Stop(useKafka bool) error {
	log.Debug("Before close", ws.clients.wsClients)
	close(ws.broadcast)
	ws.clients.mutex.RLock()
	for conn := range ws.clients.wsClients {
		ws.delConnChan <- conn
	}
	ws.clients.mutex.RUnlock()
	log.Debug("Clients list after close", ws.clients.wsClients)
	if useKafka {
		ws.wsKafka.Producer.Flush(15 * 1000)
		err := ws.wsKafka.Consumer.Close()
		if err != nil {
			log.Error(err)
		}
		ws.wsKafka.Producer.Close()
		log.Debug("Kafka producer Flush done. Producer and consumer closed")
	}
	return ws.srv.Shutdown(context.Background())
}

func (ws *wsSrv) wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := ws.wsUpg.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("Error with websocket connection: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	log.Debugf("Client with address %s connected", conn.RemoteAddr().String())

	client := newClient(conn) //&sClient{conn: conn}
	ws.connChan <- client
	go ws.readFromClient(client)
	go ws.pingLoop(client)
}

func (ws *wsSrv) addClientConn() {
	for conn := range ws.connChan {
		ws.clients.mutex.Lock()
		if _, ok := ws.clients.wsClients[conn]; !ok {
			ws.clients.wsClients[conn] = struct{}{}
			log.Debug("Создано новое соединение")
		}
		ws.clients.mutex.Unlock()
		wsActiveConnections.Inc()
	}
}

func (ws *wsSrv) delClientConn() {
	for client := range ws.delConnChan {
		ws.clients.mutex.Lock()
		if _, ok := ws.clients.wsClients[client]; ok {
			delete(ws.clients.wsClients, client)
			wsActiveConnections.Dec()
			log.Debug("Удалено соединение")
		}
		ws.clients.mutex.Unlock()
		client.Close()
	}
}

func (ws *wsSrv) readFromClient(c *sClient) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("error in read worker : %v", r)
		}
		ws.delConnChan <- c // client dead
	}()

	c.conn.SetReadLimit(512 * 1024)
	err := c.conn.SetReadDeadline(time.Now().Add(pongWait))
	if err != nil {
		log.Error(err)
	}
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		msg := new(WsMessage)
		if err := c.conn.ReadJSON(msg); err != nil {
			log.Debugf("Client disconnetced: %v", err)
			return
		}
		host, _, err := net.SplitHostPort(c.conn.RemoteAddr().String())
		if err != nil {
			log.Errorf("Error with address split: %v", err)
		}
		msg.IPAddress = host
		msg.Time = time.Now().Format("15:04")
		ws.broadcast <- msg
	}
}

func (ws *wsSrv) writeToClientsBroadCast(useKafka bool) {
	for msg := range ws.broadcast {
		ws.clients.mutex.RLock()
		sClients := make([]*sClient, 0, len(ws.clients.wsClients))
		for client := range ws.clients.wsClients {
			sClients = append(sClients, client)
		}
		ws.clients.mutex.RUnlock()

		for _, c := range sClients {
			_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := c.conn.WriteJSON(msg); err != nil {
				log.Errorf("Error with writing message: %v", err)
				ws.delConnChan <- c
			} else if useKafka {
				ws.SendKafka(msg)
			}
		}
	}
}

func (ws *wsSrv) pingLoop(c *sClient) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		ws.delConnChan <- c
	}()

	for {
		select {
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Debugf("Ping stopped: %v", err)
				return
			}
		case <-c.ctx.Done():
			return
		}
	}
}

// TODO: graceful shutdown addClientConn, delClientConn, readFromClient
// TODO: pingPong
