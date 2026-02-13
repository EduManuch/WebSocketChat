package wsserver

import (
	"encoding/json"
	"errors"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	log "github.com/sirupsen/logrus"
	"time"
)

func NewProducer(address string) (*kafka.Producer, error) {
	conf := &kafka.ConfigMap{
		"bootstrap.servers":  address,
		"acks":               "all",
		"enable.idempotence": true,
		"retries":            5,
		"linger.ms":          5,
		"batch.size":         65536, // 64 KB
		"compression.type":   "lz4",
	}
	p, err := kafka.NewProducer(conf)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (ws *wsSrv) sendToKafka(message *WsMessage) {
	if message.Host != "" {
		return
	}

	kafkaMsg := *message
	kafkaMsg.Host = ws.host

	value, err := json.Marshal(kafkaMsg)
	if err != nil {
		log.Errorf("Error while marshaling message to json: %v", err)
		return
	}

	topic := "web-topic"
	err = ws.wsKafka.Producer.Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &topic,
			Partition: kafka.PartitionAny},
		Value: value,
	}, nil)

	if err != nil {
		kafkaDropped.Inc()
		log.Errorf("Kafka produce error: %v", err)
	}
}

func (ws *wsSrv) getProducerEventsKafka() {
	for e := range ws.wsKafka.Producer.Events() {
		switch ev := e.(type) {
		case *kafka.Message:
			if ev.TopicPartition.Error != nil {
				log.Errorf("Delivery failed: %v\n", ev.TopicPartition)
			} else {
				log.Debugf("Delivered message to %v\n", ev.TopicPartition)
			}
		}
	}
}

func NewConsumer(address, hostname string) (*kafka.Consumer, error) {
	conf := &kafka.ConfigMap{
		"bootstrap.servers": address,
		"group.id":          "consumer_group_" + hostname,
		"auto.offset.reset": "latest",
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

func (ws *wsSrv) receiveKafka() {
	log.Debug("kafka consumer started")
	defer func() {
		log.Debug("kafka consumer stopping")
		_ = ws.wsKafka.Consumer.Close()
	}()

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
					kafkaDropped.Inc()
					log.Warn("Dropping kafka message: WS broadcast overloaded")
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

func (ws *wsSrv) kafkaWorker() {
	for {
		select {
		case msg, ok := <-ws.wsKafka.kafkaChan:
			if !ok {
				return
			}
			ws.sendToKafka(msg)
		case <-ws.wsKafka.ctx.Done():
			return
		}
	}
}
