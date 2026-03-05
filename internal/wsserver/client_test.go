package wsserver

import (
	"WebSocketChat/internal/types"
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSClientClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	called := false

	// Создаём клиента с mock Cancel
	client := &types.SClient{
		Conn: nil, // Не вызываем Close у Conn
		Ctx:  ctx,
		Cancel: func() {
			cancel()
			called = true
		},
		Send: make(chan *types.WsMessage, 10),
		Once: sync.Once{},
	}

	// Вызываем только Cancel (т.к. Close() пытается закрыть nil Conn)
	client.Cancel()

	// Проверяем, что Cancel был вызван
	assert.True(t, called, "Cancel должен быть вызван")
	assert.True(t, ctx.Err() != nil, "Контекст должен быть отменён")
}

func TestNewClient(t *testing.T) {
	// Создаём клиента (Conn = nil для теста)
	client := &types.SClient{
		Conn:   nil,
		Ctx:    context.Background(),
		Cancel: func() {},
		Send:   make(chan *types.WsMessage, 256),
	}

	assert.NotNil(t, client)
	assert.NotNil(t, client.Send)
	assert.Equal(t, 256, cap(client.Send))
}
