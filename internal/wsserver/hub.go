package wsserver

import (
	log "github.com/sirupsen/logrus"
	"sync"
)

type clients struct {
	mutex     *sync.RWMutex
	wsClients map[*sClient]struct{} //map[*websocket.Conn]struct{}
}

func (ws *wsSrv) addClientConn() {
	for conn := range ws.connChan {
		ws.clients.mutex.Lock()
		if _, ok := ws.clients.wsClients[conn]; !ok {
			ws.clients.wsClients[conn] = struct{}{}
			log.Debug("Создано новое соединение")
		}
		ws.clients.mutex.Unlock()
		wsActiveConnections.Inc()
	}
}

func (ws *wsSrv) delClientConn() {
	for client := range ws.delConnChan {
		ws.clients.mutex.Lock()
		if _, ok := ws.clients.wsClients[client]; ok {
			delete(ws.clients.wsClients, client)
			wsActiveConnections.Dec()
			log.Debug("Удалено соединение")
		}
		ws.clients.mutex.Unlock()
		client.Close()
	}
}

func (ws *wsSrv) readFromBroadCastWriteToClients() {
	for msg := range ws.broadcast {
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
