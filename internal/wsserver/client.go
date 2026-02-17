package wsserver

import (
	"WebSocketChat/internal/metrics"
	"WebSocketChat/internal/types"
	"context"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"net"
	"time"
)

func (ws *wsSrv) NewClient(conn *websocket.Conn) *types.SClient {
	ctx, cancel := context.WithCancel(context.Background())
	return &types.SClient{
		Conn:   conn,
		Ctx:    ctx,
		Cancel: cancel,
		Send:   make(chan *types.WsMessage, 256),
	}
}

func (ws *wsSrv) ReadFromClient(c *types.SClient) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("error in read worker : %v", r)
		}
		select {
		case ws.delConnChan <- c: // client dead
		default:
		}
	}()

	c.Conn.SetReadLimit(512 * 1024)
	err := c.Conn.SetReadDeadline(time.Now().Add(types.PongWait))
	if err != nil {
		log.Error(err)
	}
	c.Conn.SetPongHandler(func(string) error {
		return c.Conn.SetReadDeadline(time.Now().Add(types.PongWait))
	})

	for {
		var msg types.WsMessage
		if err := c.Conn.ReadJSON(&msg); err != nil {
			log.Debugf("Client disconnetced: %v", err)
			return
		}
		host, _, err := net.SplitHostPort(c.Conn.RemoteAddr().String())
		if err == nil {
			msg.IPAddress = host
		}
		msg.Time = time.Now().Format("15:04")

		select {
		case ws.broadcast <- &msg:
		case <-c.Ctx.Done():
			return
		}

		if ws.wsKafka.KChan != nil {
			select {
			case ws.wsKafka.KChan <- &msg:
			case <-ws.wsKafka.KCtx.Done():
			default:
				metrics.KafkaDropped.Inc()
				log.Warn("Kafka backlog overflow, dropping message")
			}
		}
	}
}

func (ws *wsSrv) WriteToClient(c *types.SClient) {
	ticker := time.NewTicker(types.PingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.Conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			time.Now().Add(types.WriteWait),
		)
		// не блокироваться
		select {
		case ws.delConnChan <- c:
		default:
		}
	}()

	for {
		select {
		case msg, ok := <-c.Send:
			if !ok {
				return
			}
			_ = c.Conn.SetWriteDeadline(time.Now().Add(types.WriteWait))
			if err := c.Conn.WriteJSON(msg); err != nil {
				log.Errorf("Error with writing message: %v", err)
				return
			}
		case <-ticker.C:
			_ = c.Conn.SetWriteDeadline(time.Now().Add(types.WriteWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Debugf("Ping stopped: %v", err)
				return
			}
		case <-c.Ctx.Done():
			return
		}
	}
}
