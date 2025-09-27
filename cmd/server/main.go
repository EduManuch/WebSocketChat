package main

import (
	"WebSocketChat/internal/wsserver"
	log "github.com/sirupsen/logrus"
)

const (
	addr = "192.168.221.131:8080"
)

func main() {
	wsSrv := wsserver.NewWsServer(addr)
	log.Info("Started ws server")
	if err := wsSrv.Start(); err != nil {
		log.Errorf("Error with ws server: %v", err)
	}
}
