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

func TestDelClientConn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Создаём тестовый сервер
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

	// Запускаем DelClientConn в горутине
	ws.wg.Add(1)
	go ws.DelClientConn()

	// Отправляем клиента в канал удаления (клиента нет в мапе — должна быть безопасная обработка)
	ws.delConnChan <- client

	// Даём время на обработку
	time.Sleep(50 * time.Millisecond)

	// Проверяем, что мапа пуста (не паниковало при удалении несуществующего клиента)
	ws.clients.mutex.RLock()
	count := len(ws.clients.wsClients)
	ws.clients.mutex.RUnlock()

	assert.Equal(t, 0, count, "список клиентов должен быть пуст")

	// Останавливаем сервер
	cancel()
	ws.wg.Wait()
}

func TestReadFromBroadCastWriteToClients(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Создаём тестовый сервер
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

	// Создаём тестовых клиентов с каналами для получения сообщений
	client1 := &types.SClient{
		Send: make(chan *types.WsMessage, 10),
	}
	client2 := &types.SClient{
		Send: make(chan *types.WsMessage, 10),
	}

	// Добавляем клиентов в мапу
	ws.clients.mutex.Lock()
	ws.clients.wsClients[client1] = struct{}{}
	ws.clients.wsClients[client2] = struct{}{}
	ws.clients.mutex.Unlock()

	// Запускаем ReadFromBroadCastWriteToClients в горутине
	ws.wg.Add(1)
	go ws.ReadFromBroadCastWriteToClients()

	// Отправляем тестовое сообщение в broadcast
	testMsg := &types.WsMessage{
		Message: "Hello, clients!",
		Time:    "12:00",
	}
	ws.broadcast <- testMsg

	// Даём время на обработку
	time.Sleep(50 * time.Millisecond)

	// Проверяем, что оба клиента получили сообщение
	select {
	case msg := <-client1.Send:
		assert.Equal(t, "Hello, clients!", msg.Message, "client1 должно получить сообщение")
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "client1 did not receive message")
	}

	select {
	case msg := <-client2.Send:
		assert.Equal(t, "Hello, clients!", msg.Message, "client2 должно получить сообщение")
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "client2 did not receive message")
	}

	// Останавливаем сервер
	cancel()
	ws.wg.Wait()
}
