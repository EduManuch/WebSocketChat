package kafka

import (
	"WebSocketChat/internal/metrics"
	"WebSocketChat/internal/types"
	"context"
	"encoding/json"
	"errors"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	log "github.com/sirupsen/logrus"
	"time"
)

type Kafka struct {
	Producer  *kafka.Producer
	Consumer  *kafka.Consumer
	KChan     chan *types.WsMessage
	KCtx      context.Context
	KCancel   context.CancelFunc
	KHost     string
	Broadcast chan *types.WsMessage
}

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

func (k *Kafka) SendToKafka(message *types.WsMessage) {
	if message.Host != "" {
		return
	}

	kafkaMsg := *message
	kafkaMsg.Host = k.KHost

	value, err := json.Marshal(kafkaMsg)
	if err != nil {
		log.Errorf("Error while marshaling message to json: %v", err)
		return
	}

	topic := "web-topic"
	err = k.Producer.Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &topic,
			Partition: kafka.PartitionAny},
		Value: value,
	}, nil)

	if err != nil {
		metrics.KafkaDropped.Inc()
		log.Errorf("Kafka produce error: %v", err)
	}
}

func (k *Kafka) GetProducerEventsKafka() {
	for e := range k.Producer.Events() {
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

func (k *Kafka) ReceiveKafka() {
	log.Debug("kafka consumer started")
	defer func() {
		log.Debug("kafka consumer stopping")
		_ = k.Consumer.Close()
	}()

	var ke kafka.Error
	for {
		select {
		case <-k.KCtx.Done():
			return
		default:
			message, err := k.Consumer.ReadMessage(time.Second)
			if err == nil {
				msg := new(types.WsMessage)
				err = json.Unmarshal(message.Value, msg)
				if err != nil {
					log.Errorf("Error unmarshalling kafka message: %v", err)
					continue
				}
				if msg.Host == k.KHost {
					continue
				}
				select {
				case k.Broadcast <- msg:
				case <-k.KCtx.Done():
					return
				default:
					metrics.KafkaDropped.Inc()
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

func (k *Kafka) KafkaWorker() {
	for {
		select {
		case msg, ok := <-k.KChan:
			if !ok {
				return
			}
			k.SendToKafka(msg)
		case <-k.KCtx.Done():
			return
		}
	}
}
