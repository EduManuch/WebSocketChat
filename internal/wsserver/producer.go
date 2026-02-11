package wsserver

import (
	"encoding/json"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	log "github.com/sirupsen/logrus"
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

func (ws *wsSrv) GetProducerEventsKafka() {
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
