package wsserver

import (
	"encoding/json"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	log "github.com/sirupsen/logrus"
)

func NewProducer(address string) *kafka.Producer {
	conf := &kafka.ConfigMap{"bootstrap.servers": address}
	p, err := kafka.NewProducer(conf)
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
