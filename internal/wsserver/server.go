package wsserver

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"net/http"
	"net/url"
	"os"
	"sync"
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

type Kafka struct {
	Producer  *kafka.Producer
	Consumer  *kafka.Consumer
	kafkaChan chan *WsMessage
	ctx       context.Context
	cancel    context.CancelFunc
}

func NewWsServer(e *EnvConfig) WSServer {
	m := http.NewServeMux()
	initMetrics()

	if e.Debug {
		log.SetLevel(log.DebugLevel)
	}

	hostname, _ := os.Hostname()
	var k Kafka
	if e.UseKafka {
		ctx, cancel := context.WithCancel(context.Background())
		k = Kafka{
			Producer:  NewProducer("kafka:9092"),
			Consumer:  NewConsumer("kafka:9092", hostname),
			kafkaChan: make(chan *WsMessage, 1024),
			ctx:       ctx,
			cancel:    cancel,
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

func (ws *wsSrv) Start(e *EnvConfig) error {
	ws.mux.Handle("/", http.FileServer(http.Dir(e.TemplateDir)))
	ws.mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(e.StaticDir))))
	ws.mux.HandleFunc("/ws", ws.wsHandler)
	ws.mux.Handle("/metrics", promhttp.Handler())
	go ws.addClientConn()
	go ws.delClientConn()
	go ws.readFromBroadCastWriteToClients()
	if e.UseKafka {
		go ws.GetProducerEventsKafka()
		go ws.ReceiveKafka()
		go ws.kafkaWorker()
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
	err := ws.srv.Shutdown(context.Background())

	ws.clients.mutex.RLock()
	for conn := range ws.clients.wsClients {
		select {
		case ws.delConnChan <- conn:
		default:
		}
	}
	ws.clients.mutex.RUnlock()
	log.Debug("Clients list after close", ws.clients.wsClients)

	close(ws.broadcast)

	if useKafka {
		ws.wsKafka.cancel()
		ws.wsKafka.Producer.Flush(15 * 1000)
		_ = ws.wsKafka.Consumer.Close()
		ws.wsKafka.Producer.Close()
		log.Debug("Kafka producer Flush done. Producer and consumer closed")
	}
	return err
}

func (ws *wsSrv) kafkaWorker() {
	for {
		select {
		case msg, ok := <-ws.wsKafka.kafkaChan:
			if !ok {
				return
			}
			ws.SendKafka(msg)
		case <-ws.wsKafka.ctx.Done():
			return
		}
	}
}

// TODO: graceful shutdown addClientConn, delClientConn, readFromClient
