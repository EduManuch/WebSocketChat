package types

import "time"

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
