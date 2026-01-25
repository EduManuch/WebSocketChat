package main

import (
	"WebSocketChat/internal/wsserver"
	"context"
	"github.com/joho/godotenv"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

var envs = wsserver.EnvConfig{}

func init() {
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	envs.Addr = os.Getenv("ADDR")
	envs.Port = os.Getenv("PORT")
	ssl := os.Getenv("BACKEND_SSL")
	envs.UseSsl = ssl == "enable"
	envs.CertFile = os.Getenv("CERT")
	envs.KeyFile = os.Getenv("KEY")
	envs.TemplateDir = os.Getenv("TEMPLATEDIR")
	envs.StaticDir = os.Getenv("STATICDIR")
	kafka := os.Getenv("KAFKA")
	envs.UseKafka = kafka == "enable"
	strOrigins := os.Getenv("BACKEND_ORIGINS")
	slOrigins := strings.Split(strOrigins, ",")
	envs.Origins = make(map[string]struct{})
	for _, s := range slOrigins {
		envs.Origins[s] = struct{}{}
	}
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGTERM, syscall.SIGINT)
		<-c
		cancel()
	}()

	wsSrv := wsserver.NewWsServer(&envs)
	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		log.Info("Started ws server")
		return wsSrv.Start(&envs)
	})

	g.Go(func() error {
		<-gCtx.Done()
		return wsSrv.Stop(envs.UseKafka)
	})

	if err := g.Wait(); err != nil {
		log.Printf("Shutdown server: %v\n", err)
	}
}
