package main

import (
	"WebSocketChat/internal/wsserver"
	"github.com/joho/godotenv"
	log "github.com/sirupsen/logrus"
	"os"
)

var (
	addr        string
	port        string
	certFile    string
	keyFile     string
	templateDir string
	staticDir   string
)

func init() {
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	addr = os.Getenv("ADDR")
	port = os.Getenv("PORT")
	certFile = os.Getenv("CERT")
	keyFile = os.Getenv("KEY")
	templateDir = os.Getenv("TEMPLATEDIR")
	staticDir = os.Getenv("STATICDIR")

}

func main() {
	wsSrv := wsserver.NewWsServer(addr + port)
	log.Info("Started ws server")
	if err := wsSrv.Start(certFile, keyFile, templateDir, staticDir); err != nil {
		log.Fatalf("Error with ws server: %v", err)
	}
	log.Error(wsSrv.Stop())
}
