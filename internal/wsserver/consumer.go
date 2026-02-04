package wsserver

import (
	"encoding/json"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	log "github.com/sirupsen/logrus"
	"time"
)

func NewConsumer(address, hostname string) *kafka.Consumer {
	conf := &kafka.ConfigMap{
		"bootstrap.servers": address,
		"group.id":          "consumer_group_" + hostname,
		"auto.offset.reset": "earliest",
	}
	c, err := kafka.NewConsumer(conf)
	if err != nil {
		panic(err)
	}

	err = c.Subscribe("web-topic", nil)
	if err != nil {
		panic(err)
	}

	return c
}

func (ws *wsSrv) ReceiveKafka() {
	log.Println("kafka consumer started")
	for {
		message, err := ws.wsKafka.Consumer.ReadMessage(time.Second)
		if err == nil {
			msg := new(WsMessage)
			err = json.Unmarshal(message.Value, msg)
			if err != nil {
				log.Errorf("Error unmarshalling kafka message: %v", err)
			}
			if msg.Host == ws.host {
				continue
			}
			ws.broadcast <- msg
		} else if !err.(kafka.Error).IsTimeout() {
			log.Errorf("Consumer error: %v (%v)\n", err, message)
		}
	}
}
