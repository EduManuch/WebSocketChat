package wsserver

import "time"

type WsMessage struct {
	IPAddress string `json:"address"`
	Message   string `json:"message"`
	Time      string `json:"time"`
	Host      string `json:"host"`
}

const (
	pongWait   = 10 * time.Second
	pingPeriod = 8 * time.Second
	writeWait  = 5 * time.Second
)
