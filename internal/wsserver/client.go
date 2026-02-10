package wsserver

import (
	"context"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"net"
	"sync"
	"time"
)

type sClient struct {
	conn   *websocket.Conn
	once   sync.Once
	ctx    context.Context
	cancel context.CancelFunc
	send   chan *WsMessage
}

func newClient(conn *websocket.Conn) *sClient {
	ctx, cancel := context.WithCancel(context.Background())
	return &sClient{
		conn:   conn,
		ctx:    ctx,
		cancel: cancel,
		send:   make(chan *WsMessage, 256),
	}
}

func (c *sClient) Close() {
	c.once.Do(func() {
		c.cancel()
		_ = c.conn.Close()
	})
}

func (ws *wsSrv) readFromClient(c *sClient) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("error in read worker : %v", r)
		}
		select {
		case ws.delConnChan <- c: // client dead
		default:
		}
	}()

	c.conn.SetReadLimit(512 * 1024)
	err := c.conn.SetReadDeadline(time.Now().Add(pongWait))
	if err != nil {
		log.Error(err)
	}
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		var msg WsMessage
		if err := c.conn.ReadJSON(&msg); err != nil {
			log.Debugf("Client disconnetced: %v", err)
			return
		}
		host, _, err := net.SplitHostPort(c.conn.RemoteAddr().String())
		if err == nil {
			msg.IPAddress = host
		}
		msg.Time = time.Now().Format("15:04")

		select {
		case ws.broadcast <- &msg:
		case <-c.ctx.Done():
			return
		}

		if ws.wsKafka.kafkaChan != nil {
			select {
			case ws.wsKafka.kafkaChan <- &msg:
			case <-ws.wsKafka.ctx.Done():
			default:
				kafkaDropped.Inc()
				log.Warn("Kafka backlog overflow, dropping message")
			}
		}
	}
}

func (ws *wsSrv) writeToClient(c *sClient) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			time.Now().Add(writeWait),
		)
		// не блокироваться
		select {
		case ws.delConnChan <- c:
		default:
		}
	}()

	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				return
			}
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteJSON(msg); err != nil {
				log.Errorf("Error with writing message: %v", err)
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Debugf("Ping stopped: %v", err)
				return
			}
		case <-c.ctx.Done():
			return
		}
	}

}
