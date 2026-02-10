package wsserver

import (
	"context"
	"encoding/json"
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
		panic(err)
	}
	err = createTopic(conf)
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
	}
}

func createTopic(conf *kafka.ConfigMap) error {
	admin, err := kafka.NewAdminClient(conf)
	if err != nil {
		return err
	}
	defer admin.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	_, err = admin.CreateTopics(ctx, []kafka.TopicSpecification{
		{
			Topic:             "web-topic",
			NumPartitions:     1,
			ReplicationFactor: 1,
		},
	})

	return err
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
