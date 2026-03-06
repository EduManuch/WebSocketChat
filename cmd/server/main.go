package main

import (
	"WebSocketChat/internal/types"
	"WebSocketChat/internal/wsserver"
	"context"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

var envs = types.EnvConfig{}

func init() {
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	envs.Addr = os.Getenv("ADDR")
	envs.Port = os.Getenv("PORT")
	tls := os.Getenv("BACKEND_TLS")
	envs.UseTls = tls == "enable"
	envs.CertFile = os.Getenv("CERT")
	envs.KeyFile = os.Getenv("KEY")
	envs.TemplateDir = os.Getenv("TEMPLATEDIR")
	envs.StaticDir = os.Getenv("STATICDIR")
	kafka := os.Getenv("KAFKA")
	envs.UseKafka = kafka == "enable"
	strOrigins := os.Getenv("BACKEND_ORIGINS")
	slOrigins := strings.Split(strOrigins, ",")
	envs.Origins = make(map[string]struct{})
	envs.TokenTTL = getTokenTTL()
	envs.RefreshTokenTTL = getRefreshTokenTTL()
	
	envs.JwtSecret = os.Getenv("JWT_SECRET")
	if envs.JwtSecret == "" {
		log.Fatal("JWT_SECRET is required")
	}
	
	envs.RefreshJwtSecret = os.Getenv("REFRESH_JWT_SECRET")
	if envs.JwtSecret == "" {
		log.Fatal("REFRESH_JWT_SECRET is required")
	}
	
	for _, s := range slOrigins {
		envs.Origins[s] = struct{}{}
	}
	
	debugLevel := os.Getenv("DEBUG")
	if debugLevel == "enable" {
		log.SetLevel(log.DebugLevel)
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

	wsSrv, err := wsserver.NewWsServer(&envs)
	if err != nil {
		return
	}
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
		log.Infof("Shutdown server: %v\n", err)
	}
}

func getTokenTTL() time.Duration{
	tokenTTLStr := os.Getenv("TOKEN_TTL")
	tokenTTL, err := strconv.Atoi(tokenTTLStr)
	if err != nil {
		log.Fatalf("Invalid TOKEN_TTL value '%s': %v", tokenTTLStr, err)
	}
	return time.Duration(tokenTTL) * time.Second
}

func getRefreshTokenTTL() time.Duration{
	tokenTTLStr := os.Getenv("REFRESH_TOKEN_TTL")
	tokenTTL, err := strconv.Atoi(tokenTTLStr)
	if err != nil {
		log.Fatalf("Invalid TOKEN_TTL value '%s': %v", tokenTTLStr, err)
	}
	day := time.Hour * 24
	return time.Duration(tokenTTL) * day
}


// TODO: refresh token rotation
// TODO: logout