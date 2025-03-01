package network

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type PrometheusMetrics struct {
	connections prometheus.Gauge
	errors      prometheus.Counter
	requests    prometheus.Counter
}

func NewPrometheusMetrics() *PrometheusMetrics {
	m := &PrometheusMetrics{
		connections: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "server_active_connections",
			Help: "Current number of active connections.",
		}),
		errors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "server_errors_total",
			Help: "Total number of errors.",
		}),
		requests: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "server_requests_total",
			Help: "Total number of requests handled.",
		}),
	}

	// Register the metrics
	prometheus.MustRegister(m.connections, m.errors, m.requests)
	return m
}

func (m *PrometheusMetrics) IncConnections() {
	m.connections.Inc()
}

func (m *PrometheusMetrics) DecConnections() {
	m.connections.Dec()
}

func (m *PrometheusMetrics) IncErrors() {
	m.errors.Inc()
}

func (m *PrometheusMetrics) IncRequests() {
	m.requests.Inc()
}

// Start a metrics HTTP server
func StartMetricsServer() {
	http.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(":8081", nil)
}
