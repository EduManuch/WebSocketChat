package wsserver

type WsMessage struct {
	IPAddress string `json:"address"`
	Message   string `json:"message"`
	Time      string `json:"time"`
	FromKafka bool   `json:"fromKafka"`
}
