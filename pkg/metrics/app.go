package metrics

import (
	"context"
	"runtime"
)

var durationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// Default is the registry served at /metrics.
var Default = NewRegistry()

var (
	HTTPRequests = Default.Counter("sati_http_requests_total", "HTTP requests handled.", "method", "route", "status")
	HTTPDuration = Default.Histogram("sati_http_request_duration_seconds", "HTTP request duration.", durationBuckets, "method", "route")
	RateLimited  = Default.Counter("sati_rate_limited_total", "Requests rejected by a rate limit.", "policy")
	Panics       = Default.Counter("sati_panics_total", "Handler panics recovered.")

	OrdersPlaced       = Default.Counter("sati_orders_placed_total", "Orders created.", "payment_method", "fulfillment")
	OrderStatusChanges = Default.Counter("sati_order_status_changes_total", "Order status changes.", "status")
	Payments           = Default.Counter("sati_payments_total", "Online payment outcomes.", "provider", "result")
	Webhooks           = Default.Counter("sati_webhook_events_total", "Payment webhook events.", "provider", "result")
	Emails             = Default.Counter("sati_emails_total", "Email send attempts.", "result")
)

func init() {
	Default.GaugeFunc("sati_goroutines", "Running goroutines.", nil, func(context.Context) ([]Sample, error) {
		return []Sample{{Value: float64(runtime.NumGoroutine())}}, nil
	})
	Default.GaugeFunc("sati_memory_alloc_bytes", "Heap memory in use.", nil, func(context.Context) ([]Sample, error) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return []Sample{{Value: float64(m.Alloc)}}, nil
	})
}
