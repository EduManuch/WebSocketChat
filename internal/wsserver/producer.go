package wsserver

import (
	"context"
	"encoding/json"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	log "github.com/sirupsen/logrus"
	"time"
)

func NewProducer(address string) *kafka.Producer {
	conf := &kafka.ConfigMap{"bootstrap.servers": address}
	p, err := kafka.NewProducer(conf)
	if err != nil {
		panic(err)
	}
	err = createTopic(conf)
	if err != nil {
		panic(err)
	}
	return p
}

func (ws *wsSrv) SendKafka(message *WsMessage) {
	if message.Host == "" {
		message.Host = ws.host
		value, err := json.Marshal(message)
		if err != nil {
			log.Errorf("Error while marshaling message to json: %v", err)
		}
		topic := "web-topic"
		ws.wsKafka.Producer.Produce(&kafka.Message{
			TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: kafka.PartitionAny},
			Value:          value,
		}, nil)

		ws.wsKafka.Producer.Flush(15 * 1000)
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
				log.Printf("Delivery failed: %v\n", ev.TopicPartition)
			} else {
				log.Printf("Delivered message to %v\n", ev.TopicPartition)
			}
		}
	}
}
