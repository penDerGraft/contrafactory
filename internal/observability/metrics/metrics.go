// Package metrics provides Prometheus instrumentation for contrafactory.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	enabled     bool
	serviceName string

	// HTTP metrics
	httpRequestsTotal *prometheus.CounterVec
	httpDuration      *prometheus.HistogramVec

	// Package domain metrics
	packagePublishTotal  *prometheus.CounterVec
	packageRetrieveTotal *prometheus.CounterVec
	packageDeleteTotal   *prometheus.CounterVec

	// Deployment domain metrics
	deploymentRecordTotal *prometheus.CounterVec
	deploymentVerifyTotal *prometheus.CounterVec

	// Verification domain metrics
	verificationTotal *prometheus.CounterVec
)

// Init initializes the metrics system.
func Init(enabledFlag bool, svcName string) {
	enabled = enabledFlag
	serviceName = svcName

	if !enabled {
		return
	}

	// HTTP request counter
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	// HTTP request duration histogram
	httpDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	// Package publish counter
	packagePublishTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "package_publish_total",
			Help: "Total number of packages published",
		},
		[]string{"chain", "builder", "status"},
	)

	// Package retrieve counter
	packageRetrieveTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "package_retrieve_total",
			Help: "Total number of packages retrieved",
		},
		[]string{"status"},
	)

	// Package delete counter
	packageDeleteTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "package_delete_total",
			Help: "Total number of packages deleted",
		},
		[]string{"status"},
	)

	// Deployment record counter
	deploymentRecordTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "deployment_record_total",
			Help: "Total number of deployments recorded",
		},
		[]string{"chain", "status"},
	)

	// Deployment verify counter
	deploymentVerifyTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "deployment_verify_total",
			Help: "Total number of deployment verification updates",
		},
		[]string{"status"},
	)

	// Verification request counter
	verificationTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "verification_request_total",
			Help: "Total number of verification requests",
		},
		[]string{"result"},
	)

	// Note: Go runtime metrics (goroutines, memory, GC) are automatically
	// collected by prometheus/client_golang - no custom collector needed
}

// Handler returns the Prometheus metrics HTTP handler.
func Handler() http.Handler {
	if !enabled {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
	}
	return promhttp.Handler()
}

// Enabled returns whether metrics are enabled.
func Enabled() bool {
	return enabled
}

// ServiceName returns the configured service name for metric labels.
func ServiceName() string {
	return serviceName
}
