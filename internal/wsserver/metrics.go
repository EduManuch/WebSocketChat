package wsserver

import "github.com/prometheus/client_golang/prometheus"

var (
	wsActiveConnections = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ws_active_connections",
			Help: " Websocket active connections",
		},
	)

	kafkaDropped = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "kafka_dropped_messages_total",
			Help: "Dropped Kafka messages due to backpressure",
		})
)

func initMetrics() {
	prometheus.MustRegister(wsActiveConnections)
	prometheus.MustRegister(kafkaDropped)
	wsActiveConnections.Set(0)
}
