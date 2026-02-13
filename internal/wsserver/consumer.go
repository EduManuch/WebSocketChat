package wsserver

import (
	"encoding/json"
	"errors"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	log "github.com/sirupsen/logrus"
	"time"
)

func NewConsumer(address, hostname string) (*kafka.Consumer, error) {
	conf := &kafka.ConfigMap{
		"bootstrap.servers": address,
		"group.id":          "consumer_group_" + hostname,
		"auto.offset.reset": "earliest",
	}
	c, err := kafka.NewConsumer(conf)
	if err != nil {
		return nil, err
	}

	err = c.Subscribe("web-topic", nil)
	if err != nil {
		return nil, err
	}

	return c, nil
}

func (ws *wsSrv) ReceiveKafka() {
	log.Debug("kafka consumer started")
	var ke kafka.Error
	for {
		select {
		case <-ws.wsKafka.ctx.Done():
		default:
			message, err := ws.wsKafka.Consumer.ReadMessage(time.Second)
			if err == nil {
				msg := new(WsMessage)
				err = json.Unmarshal(message.Value, msg)
				if err != nil {
					log.Errorf("Error unmarshalling kafka message: %v", err)
					continue
				}
				if msg.Host == ws.host {
					continue
				}
				select {
				case ws.broadcast <- msg:
				case <-ws.wsKafka.ctx.Done():
					return
				default:
					log.Error("Kafka consumer error sending message to broadcast channel")
				}
			} else if errors.As(err, &ke) {
				if !ke.IsTimeout() {
					log.Errorf("Consumer error: %v (%v)\n", err, message)
				}
			} else {
				log.Errorf("Non-kafka error: %v", err)
			}
		}
	}
}
