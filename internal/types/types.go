package types

import (
	"context"
	"github.com/gorilla/websocket"
	"sync"
	"time"
)

type WsServer interface {
	NewClient(*websocket.Conn) *SClient
	Start(*EnvConfig) error
	Stop(bool) error
	AddClientConn()
	DelClientConn()
	ReadFromBroadCastWriteToClients()
	ReadFromClient(*SClient)
	WriteToClient(*SClient)
	GetConnChan() chan *SClient
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

type SClient struct {
	Conn   *websocket.Conn
	Once   sync.Once
	Ctx    context.Context
	Cancel context.CancelFunc
	Send   chan *WsMessage
}

func (c *SClient) Close() {
	c.Once.Do(func() {
		c.Cancel()
		_ = c.Conn.Close()
	})
}

type WsMessage struct {
	IPAddress string `json:"address"`
	Message   string `json:"message"`
	Time      string `json:"time"`
	Host      string `json:"host"`
}

const (
	PongWait   = 10 * time.Second
	PingPeriod = 8 * time.Second
	WriteWait  = 5 * time.Second
)
