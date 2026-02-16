package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	WsActiveConnections = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ws_active_connections",
			Help: " Websocket active connections",
		},
	)

	KafkaDropped = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "kafka_dropped_messages_total",
			Help: "Dropped Kafka messages due to backpressure",
		})
)

func InitMetrics() {
	prometheus.MustRegister(WsActiveConnections)
	prometheus.MustRegister(KafkaDropped)
	WsActiveConnections.Set(0)
}
