package wsserver

import (
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"log"
	"time"
)

func NewConsumer(address string) *kafka.Consumer {
	conf := &kafka.ConfigMap{
		"bootstrap.servers": address,
		"group.id":          "consumer_group1",
		"auto.offset.reset": "earliest",
	}
	c, err := kafka.NewConsumer(conf)
	if err != nil {
		panic(err)
	}

	return c
}

func doConsume() {
	c := NewConsumer("localhost:9092")
	err := c.Subscribe("websock-topic1", nil)
	if err != nil {
		panic(err)
	}

	for {
		msg, err := c.ReadMessage(time.Second)
		if err == nil {
			log.Printf("Message on %s: %s\n", msg.TopicPartition, string(msg.Value))
		} else if !err.(kafka.Error).IsTimeout() {
			log.Printf("Consumer error: %v (%v)\n", err, msg)
		}
	}
}
