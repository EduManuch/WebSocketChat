package types

import (
	"context"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
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
	JwtSecret   string
	TokenTTL    time.Duration
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

type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token string            `json:"token"`
	User  UserLoginResponse `json:"user"`
}

type UserLoginResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type RegisterResponse struct {
	Message string `json:"message"`
}

type Claims struct {
	UserID   string
	Username string
	jwt.RegisteredClaims
}

type ErrorResponse struct {
	Message string `json:"message"`
}

const (
	PongWait   = 10 * time.Second
	PingPeriod = 8 * time.Second
	WriteWait  = 5 * time.Second
)
