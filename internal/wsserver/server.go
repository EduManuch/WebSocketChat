package wsserver

import (
	"WebSocketChat/internal/auth"
	"WebSocketChat/internal/handlers"
	wskafka "WebSocketChat/internal/kafka"
	"WebSocketChat/internal/metrics"
	"WebSocketChat/internal/types"
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

type wsSrv struct {
	mux         *http.ServeMux
	srv         *http.Server
	wsHandler   handlers.WsHandler
	broadcast   chan *types.WsMessage
	clients     clients
	clientsWg   sync.WaitGroup
	connChan    chan *types.SClient
	delConnChan chan *types.SClient
	wsKafka     wskafka.Kafka
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

func NewWsServer(e *types.EnvConfig) (types.WsServer, error) {
	m := http.NewServeMux()
	metrics.InitMetrics()
	broadcastChan := make(chan *types.WsMessage, 1024)
	addr := e.Addr + e.Port
	if e.Debug {
		log.SetLevel(log.DebugLevel)
	}
	wsHandler := handlers.WsHandler{
		Upgrader: &websocket.Upgrader{
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
		AuthService: auth.NewService(e.JwtSecret, e.TokenTTL),
	}

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
	return &wsSrv{
		mux: m,
		srv: &http.Server{
			Addr:    addr,
			Handler: m,
		},
		wsHandler: wsHandler,
		broadcast: broadcastChan,
		clients: clients{
			mutex:     sync.RWMutex{},
			wsClients: make(map[*types.SClient]struct{}, 1024),
		},
		connChan:    make(chan *types.SClient, 1024),
		delConnChan: make(chan *types.SClient, 1024),
		wsKafka:     k,
		ctx:         ctx,
		cancel:      cancel,
		wg:          sync.WaitGroup{},
	}, nil
}

func (ws *wsSrv) GetConnChan() chan *types.SClient {
	return ws.connChan
}

func (ws *wsSrv) Start(e *types.EnvConfig) error {
	ws.mux.HandleFunc("/auth/login", func(w http.ResponseWriter, r *http.Request) {
		ws.wsHandler.LoginUser(w, r, e)
	})
	ws.mux.HandleFunc("/auth/register", func(w http.ResponseWriter, r *http.Request) {
		ws.wsHandler.RegisterUser(w, r, e)
	})
	ws.mux.HandleFunc("/auth/me", ws.wsHandler.AuthService.JWTMiddleware(ws.wsHandler.Me))
	ws.mux.HandleFunc("/ws", ws.wsHandler.AuthService.JWTMiddleware(func(w http.ResponseWriter, r *http.Request) {
		ws.wsHandler.CreateWsConnection(w, r, ws)
	}))

	ws.mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(e.StaticDir))))
	ws.mux.Handle("/metrics", promhttp.Handler())
	ws.mux.Handle("/", http.FileServer(http.Dir(e.TemplateDir)))
	ws.wg.Add(3)
	go ws.AddClientConn()
	go ws.DelClientConn()
	go ws.ReadFromBroadCastWriteToClients()
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
