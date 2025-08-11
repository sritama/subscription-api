package telemetry

import (
	"context"
	"fmt"
	"time"

	"scalable-paywall/internal/config"

	"github.com/gin-gonic/gin"
	prometheusClient "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	otelPrometheus "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

var (
	httpRequestsTotal = prometheusClient.NewCounterVec(
		prometheusClient.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "endpoint", "status"},
	)

	httpRequestDuration = prometheusClient.NewHistogramVec(
		prometheusClient.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheusClient.DefBuckets,
		},
		[]string{"method", "endpoint"},
	)

	subscriptionOperations = prometheusClient.NewCounterVec(
		prometheusClient.CounterOpts{
			Name: "subscription_operations_total",
			Help: "Total number of subscription operations",
		},
		[]string{"operation", "status"},
	)

	paywallChecks = prometheusClient.NewCounterVec(
		prometheusClient.CounterOpts{
			Name: "paywall_checks_total",
			Help: "Total number of paywall checks",
		},
		[]string{"result"},
	)

	paymentOperations = prometheusClient.NewCounterVec(
		prometheusClient.CounterOpts{
			Name: "payment_operations_total",
			Help: "Total number of payment operations",
		},
		[]string{"operation", "status"},
	)

	userOperations = prometheusClient.NewCounterVec(
		prometheusClient.CounterOpts{
			Name: "user_operations_total",
			Help: "Total number of user operations",
		},
		[]string{"operation", "status"},
	)

	planOperations = prometheusClient.NewCounterVec(
		prometheusClient.CounterOpts{
			Name: "plan_operations_total",
			Help: "Total number of plan operations",
		},
		[]string{"operation", "status"},
	)
)

func init() {
	prometheusClient.MustRegister(httpRequestsTotal)
	prometheusClient.MustRegister(httpRequestDuration)
	prometheusClient.MustRegister(subscriptionOperations)
	prometheusClient.MustRegister(paywallChecks)
	prometheusClient.MustRegister(paymentOperations)
	prometheusClient.MustRegister(userOperations)
	prometheusClient.MustRegister(planOperations)
}

type Provider struct {
	metricsProvider *metric.MeterProvider
}

func InitProvider(cfg config.TelemetryConfig) (*Provider, error) {
	if !cfg.Enabled {
		return &Provider{}, nil
	}

	// Create resource
	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.Version),
			semconv.DeploymentEnvironment(cfg.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create Prometheus exporter
	exporter, err := otelPrometheus.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create Prometheus exporter: %w", err)
	}

	// Create meter provider
	metricsProvider := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(exporter),
	)

	// Set global meter provider
	otel.SetMeterProvider(metricsProvider)

	return &Provider{
		metricsProvider: metricsProvider,
	}, nil
}

func (p *Provider) Shutdown(ctx context.Context) error {
	if p.metricsProvider != nil {
		return p.metricsProvider.Shutdown(ctx)
	}
	return nil
}

func GinMiddleware() gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		start := time.Now()

		// Process request
		c.Next()

		// Record metrics
		duration := time.Since(start).Seconds()
		status := fmt.Sprintf("%d", c.Writer.Status())
		method := c.Request.Method
		endpoint := c.FullPath()
		if endpoint == "" {
			endpoint = c.Request.URL.Path
		}

		httpRequestsTotal.WithLabelValues(method, endpoint, status).Inc()
		httpRequestDuration.WithLabelValues(method, endpoint).Observe(duration)
	})
}

func MetricsHandler() gin.HandlerFunc {
	return gin.WrapH(promhttp.Handler())
}

// Helper functions for recording business metrics
func RecordSubscriptionOperation(operation, status string) {
	subscriptionOperations.WithLabelValues(operation, status).Inc()
}

func RecordPaywallCheck(result string) {
	paywallChecks.WithLabelValues(result).Inc()
}

func RecordPaymentOperation(operation, status string) {
	paymentOperations.WithLabelValues(operation, status).Inc()
}

func RecordUserOperation(operation, status string) {
	userOperations.WithLabelValues(operation, status).Inc()
}

func RecordPlanOperation(operation, status string) {
	planOperations.WithLabelValues(operation, status).Inc()
}
