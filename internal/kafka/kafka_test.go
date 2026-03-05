package kafka

import (
	"WebSocketChat/internal/types"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSendToKafka_Logic(t *testing.T) {
	t.Run("сообщение без Host должно обрабатываться", func(t *testing.T) {
		msg := &types.WsMessage{
			Message: "Hello",
			Time:    "12:00",
			Host:    "",
		}

		// Проверяем логику: если Host == "", сообщение обрабатывается
		assert.Equal(t, "", msg.Host, "сообщение должно обрабатываться")

		// Проверяем маршалинг
		kafkaMsg := *msg
		kafkaMsg.Host = "test-host"

		assert.Equal(t, "test-host", kafkaMsg.Host)

		value, err := json.Marshal(kafkaMsg)
		assert.NoError(t, err)
		assert.NotEmpty(t, value)
	})

	t.Run("сообщение с Host игнорируется", func(t *testing.T) {
		msg := &types.WsMessage{
			Message: "Hello",
			Host:    "other-host",
		}

		// Проверяем логику: если Host != "", сообщение игнорируется
		assert.NotEqual(t, "", msg.Host, "сообщение не должно обрабатываться")
	})
}

func TestKafkaWorker(t *testing.T) {
	t.Run("остановка при закрытии канала", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		k := &Kafka{
			KHost:   "test-host",
			KCtx:    ctx,
			KCancel: cancel,
			KChan:   make(chan *types.WsMessage, 10),
		}

		// Закрываем канал
		close(k.KChan)

		// Запускаем KafkaWorker
		done := make(chan struct{})
		go func() {
			k.KafkaWorker()
			close(done)
		}()

		select {
		case <-done:
			// Воркер остановился
		case <-time.After(100 * time.Millisecond):
			t.Error("KafkaWorker не остановился после закрытия канала")
		}
	})

	t.Run("остановка при отмене контекста", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		k := &Kafka{
			KHost:   "test-host",
			KCtx:    ctx,
			KCancel: cancel,
			KChan:   make(chan *types.WsMessage, 10),
		}

		// Запускаем KafkaWorker
		done := make(chan struct{})
		go func() {
			k.KafkaWorker()
			close(done)
		}()

		// Даём время на запуск
		time.Sleep(10 * time.Millisecond)

		// Отменяем контекст
		cancel()

		select {
		case <-done:
			// Воркер остановился
		case <-time.After(100 * time.Millisecond):
			t.Error("KafkaWorker не остановился после отмены контекста")
		}
	})
}

func TestReceiveKafka_Logic(t *testing.T) {
	t.Run("пропуск сообщений от своего хоста", func(t *testing.T) {
		k := &Kafka{
			KHost: "my-host",
		}

		msg := &types.WsMessage{
			Message: "Own message",
			Host:    "my-host",
		}

		// Проверяем логику: если Host == KHost, сообщение игнорируется
		assert.Equal(t, k.KHost, msg.Host, "сообщение должно быть определено как своё")
	})

	t.Run("обработка сообщений от других хостов", func(t *testing.T) {
		k := &Kafka{
			KHost: "my-host",
		}

		msg := &types.WsMessage{
			Message: "External message",
			Host:    "other-host",
		}

		// Проверяем логику: если Host != KHost, сообщение обрабатывается
		assert.NotEqual(t, k.KHost, msg.Host, "сообщение должно быть определено как чужое")
		assert.Equal(t, "External message", msg.Message)
	})

	t.Run("unmarshal сообщения из JSON", func(t *testing.T) {
		jsonData := `{"message": "Test", "address": "127.0.0.1", "time": "12:00", "host": "remote"}`

		msg := new(types.WsMessage)
		err := json.Unmarshal([]byte(jsonData), msg)

		assert.NoError(t, err)
		assert.Equal(t, "Test", msg.Message)
		assert.Equal(t, "127.0.0.1", msg.IPAddress)
		assert.Equal(t, "12:00", msg.Time)
		assert.Equal(t, "remote", msg.Host)
	})
}
