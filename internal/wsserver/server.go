package wsserver

import (
	wskafka "WebSocketChat/internal/kafka"
	"WebSocketChat/internal/metrics"
	"WebSocketChat/internal/types"
	"context"
	"crypto/tls"
	"fmt"
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
	broadcast   chan *types.WsMessage
	clients     clients
	clientsWg   sync.WaitGroup
	connChan    chan *sClient
	delConnChan chan *sClient
	wsKafka     wskafka.Kafka
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

func NewWsServer(e *EnvConfig) (WSServer, error) {
	m := http.NewServeMux()
	metrics.InitMetrics()

	if e.Debug {
		log.SetLevel(log.DebugLevel)
	}

	broadcastChan := make(chan *types.WsMessage, 1024)
	var k wskafka.Kafka
	if e.UseKafka {
		hostname, _ := os.Hostname()
		producer, err := wskafka.NewProducer("kafka:9092")
		if err != nil {
			log.Errorf("New producer error: %v", err)
			return nil, err
		}
		consumer, err := wskafka.NewConsumer("kafka:9092", hostname)
		if err != nil {
			log.Errorf("New consumer error: %v", err)
			return nil, err
		}

		KCtx, KCancel := context.WithCancel(context.Background())
		k = wskafka.Kafka{
			Producer:  producer,
			Consumer:  consumer,
			KChan:     make(chan *types.WsMessage, 1024),
			KCtx:      KCtx,
			KCancel:   KCancel,
			KHost:     hostname,
			Broadcast: broadcastChan,
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
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
		broadcast: broadcastChan,
		clients: clients{
			mutex:     &sync.RWMutex{},
			wsClients: make(map[*sClient]struct{}, 1024),
		},
		connChan:    make(chan *sClient, 1024),
		delConnChan: make(chan *sClient, 1024),
		wsKafka:     k,
		ctx:         ctx,
		cancel:      cancel,
		wg:          sync.WaitGroup{},
	}, nil
}

func (ws *wsSrv) Start(e *EnvConfig) error {
	ws.mux.Handle("/", http.FileServer(http.Dir(e.TemplateDir)))
	ws.mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(e.StaticDir))))
	ws.mux.HandleFunc("/ws", ws.wsHandler)
	ws.mux.Handle("/metrics", promhttp.Handler())
	ws.wg.Add(3)
	go ws.addClientConn()
	go ws.delClientConn()
	go ws.readFromBroadCastWriteToClients()
	if e.UseKafka {
		go ws.wsKafka.GetProducerEventsKafka()
		go ws.wsKafka.ReceiveKafka()
		go ws.wsKafka.KafkaWorker()
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

	if useKafka {
		ws.wsKafka.KCancel()
	}

	ws.clients.mutex.RLock()
	for conn := range ws.clients.wsClients {
		ws.delConnChan <- conn
	}
	ws.clients.mutex.RUnlock()
	ws.clientsWg.Wait()
	log.Debug("Clients list after close", ws.clients.wsClients)
	ws.cancel()
	ws.wg.Wait()

	if useKafka {
		ws.wsKafka.Producer.Flush(15_000)
		_ = ws.wsKafka.Consumer.Close()
		ws.wsKafka.Producer.Close()
		log.Debug("Kafka producer Flush done. Producer and consumer closed")
	}
	return err
}
