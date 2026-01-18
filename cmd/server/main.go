package main

import (
	"WebSocketChat/internal/wsserver"
	"context"
	"github.com/joho/godotenv"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"os"
	"os/signal"
	"syscall"
)

var (
	addr        string
	port        string
	certFile    string
	keyFile     string
	templateDir string
	staticDir   string
	useKafka    bool
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
	kafka := os.Getenv("KAFKA")
	useKafka = kafka == "enable"
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGTERM, syscall.SIGINT)
		<-c
		cancel()
	}()

	wsSrv := wsserver.NewWsServer(addr+port, useKafka)
	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		log.Info("Started ws server")
		return wsSrv.Start(certFile, keyFile, templateDir, staticDir, useKafka)
	})

	g.Go(func() error {
		<-gCtx.Done()
		return wsSrv.Stop(useKafka)
	})

	if err := g.Wait(); err != nil {
		log.Printf("Shutdown server: %v\n", err)
	}
}
