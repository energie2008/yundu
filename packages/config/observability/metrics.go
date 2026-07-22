package observability

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const ServiceLabel = "service"

var (
	RequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total number of HTTP requests completed.",
	}, []string{"service", "method", "path", "status"})

	RequestDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request latency in seconds.",
		Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
	}, []string{"service", "method", "path"})

	RequestsInFlight = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "http_requests_in_flight",
		Help: "Current number of HTTP requests being processed.",
	}, []string{"service"})

	PanicsRecoveredTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_panics_recovered_total",
		Help: "Total number of HTTP panics recovered by middleware.",
	}, []string{"service"})

	BuildInfo = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "app_build_info",
		Help: "Application build info.",
	}, []string{"service", "version", "commit"})
)

func PrometheusMiddleware(serviceName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		RequestsInFlight.WithLabelValues(serviceName).Inc()
		defer RequestsInFlight.WithLabelValues(serviceName).Dec()

		c.Next()

		status := strconv.Itoa(c.Writer.Status())
		duration := time.Since(start).Seconds()
		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}

		RequestsTotal.With(prometheus.Labels{
			"service": serviceName,
			"method":  c.Request.Method,
			"path":    path,
			"status":  status,
		}).Inc()
		RequestDurationSeconds.With(prometheus.Labels{
			"service": serviceName,
			"method":  c.Request.Method,
			"path":    path,
		}).Observe(duration)
	}
}

func RecordPanic(serviceName string) {
	PanicsRecoveredTotal.WithLabelValues(serviceName).Inc()
}

func SetBuildInfo(serviceName, version, commit string) {
	BuildInfo.WithLabelValues(serviceName, version, commit).Set(1)
}
