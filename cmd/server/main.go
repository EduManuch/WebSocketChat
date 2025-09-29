package main

import (
	"WebSocketChat/internal/wsserver"
	log "github.com/sirupsen/logrus"
)

const (
	addr     = "127.0.0.1:8443"
	certFile = "certs/cert.crt"
	keyFile  = "certs/key.pem"
)

func main() {
	wsSrv := wsserver.NewWsServer(addr)
	log.Info("Started ws server")
	if err := wsSrv.Start(certFile, keyFile); err != nil {
		log.Errorf("Error with ws server: %v", err)
	}
	log.Error(wsSrv.Stop())
}
