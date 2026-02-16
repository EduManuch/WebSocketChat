package wsserver

import (
	"WebSocketChat/internal/metrics"
	log "github.com/sirupsen/logrus"
	"sync"
)

type clients struct {
	mutex     *sync.RWMutex
	wsClients map[*sClient]struct{} //map[*websocket.Conn]struct{}
}

func (ws *wsSrv) addClientConn() {
	defer ws.wg.Done()

	for {
		select {
		case <-ws.ctx.Done():
			return
		case conn, ok := <-ws.connChan:
			if !ok {
				return
			}
			ws.clients.mutex.Lock()
			if _, ok := ws.clients.wsClients[conn]; !ok {
				ws.clients.wsClients[conn] = struct{}{}
				log.Debug("Создано новое соединение")
				ws.clientsWg.Add(1)
			}
			ws.clients.mutex.Unlock()
			metrics.WsActiveConnections.Inc()
		}
	}
}

func (ws *wsSrv) delClientConn() {
	defer ws.wg.Done()

	for {
		select {
		case <-ws.ctx.Done():
			return
		case client, ok := <-ws.delConnChan:
			if !ok {
				return
			}
			ws.clients.mutex.Lock()
			if _, ok := ws.clients.wsClients[client]; ok {
				delete(ws.clients.wsClients, client)
				metrics.WsActiveConnections.Dec()
				log.Debug("Удалено соединение")
				client.Close()
				ws.clientsWg.Done()
			}
			ws.clients.mutex.Unlock()
		}
	}
}

func (ws *wsSrv) readFromBroadCastWriteToClients() {
	defer ws.wg.Done()

	for {
		select {
		case <-ws.ctx.Done():
			return
		case msg, ok := <-ws.broadcast:
			if !ok {
				return
			}
			ws.clients.mutex.RLock()
			sClients := make([]*sClient, 0, len(ws.clients.wsClients))
			for client := range ws.clients.wsClients {
				sClients = append(sClients, client)
			}
			ws.clients.mutex.RUnlock()

			for _, c := range sClients {
				select {
				case c.send <- msg:
				default:
					select {
					case ws.delConnChan <- c:
						log.Debugf("Error readFromBroadCastWriteToClients. Slow client")
					default:
					}
				}
			}
		}
	}
}
