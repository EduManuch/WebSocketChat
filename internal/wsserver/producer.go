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

func (k *Kafka) SendKafka(message *WsMessage) {
	go func() {
		for e := range k.Producer.Events() {
			switch ev := e.(type) {
			case *kafka.Message:
				if ev.TopicPartition.Error != nil {
					log.Printf("Delivery failed: %v\n", ev.TopicPartition)
				} else {
					log.Printf("Delivered message to %v\n", ev.TopicPartition)
				}
			}
		}
	}()

	if !message.FromKafka {
		message.FromKafka = true
		value, err := json.Marshal(message)
		if err != nil {
			log.Errorf("Error while marshaling message to json: %v", err)
		}
		topic := "web-topic1"
		k.Producer.Produce(&kafka.Message{
			TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: kafka.PartitionAny},
			Value:          value,
		}, nil)

		k.Producer.Flush(15 * 1000)
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
			Topic:             "web-topic1",
			NumPartitions:     1,
			ReplicationFactor: 1,
		},
	})

	return err
}
