package wsserver

import (
	"WebSocketChat/internal/types"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAddClientConn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Создаём тестовый сервер с необходимыми полями
	ws := &wsSrv{
		connChan:    make(chan *types.SClient, 10),
		delConnChan: make(chan *types.SClient, 10),
		broadcast:   make(chan *types.WsMessage, 10),
		ctx:         ctx,
		wg:          sync.WaitGroup{},
		clientsWg:   sync.WaitGroup{},
		clients: clients{
			mutex:     sync.RWMutex{},
			wsClients: make(map[*types.SClient]struct{}),
		},
	}

	// Создаём тестового клиента
	client := &types.SClient{
		Send: make(chan *types.WsMessage, 10),
	}

	// Запускаем AddClientConn в горутине
	ws.wg.Add(1)
	go ws.AddClientConn()

	// Отправляем клиента в канал
	ws.connChan <- client

	// Даём время на обработку
	time.Sleep(50 * time.Millisecond)

	// Проверяем, что клиент добавлен
	ws.clients.mutex.RLock()
	_, exists := ws.clients.wsClients[client]
	ws.clients.mutex.RUnlock()

	assert.True(t, exists, "клиент должен быть добавлен")
	assert.Equal(t, 1, len(ws.clients.wsClients), "должен быть один клиент")

	// Останавливаем сервер
	cancel()
	ws.wg.Wait()
}
