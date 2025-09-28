package main

import (
	"WebSocketChat/internal/wsserver"
	log "github.com/sirupsen/logrus"
)

const (
	addr = "127.0.0.1:8080"
)

func main() {
	wsSrv := wsserver.NewWsServer(addr)
	log.Info("Started ws server")
	if err := wsSrv.Start(); err != nil {
		log.Errorf("Error with ws server: %v", err)
	}
	log.Error(wsSrv.Stop())

	//go func() {
	//	if err := wsSrv.Start(); err != nil {
	//		log.Errorf("Error with ws server: %v", err)
	//	}
	//}()
	//time.Sleep(time.Second * 5)
	//log.Error(wsSrv.Stop())
}
