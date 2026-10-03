package metrics

import (
	"strconv"
	"strings"
	"time"
)

type Registry struct {
	requestTotal    *counterVec
	requestDuration *histogramVec
	bookingTotal    *counterVec
	dbErrors        *counterVec
	rateLimited     *counterVec
	buildInfo       *gauge
}

var durationBuckets = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}

func New() *Registry {
	return &Registry{
		requestTotal: newCounterVec(
			"booking_api_http_requests_total",
			"Total number of HTTP requests handled, by route, method and status.",
			[]string{"route", "method", "status"},
		),
		requestDuration: newHistogramVec(
			"booking_api_http_request_duration_seconds",
			"HTTP request latency in seconds, by route and method.",
			[]string{"route", "method"},
			durationBuckets,
		),
		bookingTotal: newCounterVec(
			"booking_api_booking_operations_total",
			"Booking operations by kind (created, series_created, cancelled, conflict) and outcome.",
			[]string{"kind", "outcome"},
		),
		dbErrors: newCounterVec(
			"booking_api_database_errors_total",
			"Database errors by SQLSTATE class.",
			[]string{"sqlstate"},
		),
		rateLimited: newCounterVec(
			"booking_api_rate_limited_total",
			"Requests rejected by the rate limiter.",
			[]string{"route"},
		),
		buildInfo: newGauge(
			"booking_api_build_info",
			"Build information of the running binary.",
			[]string{"version"},
		),
	}
}

func (r *Registry) ObserveRequest(route, method string, status int, d time.Duration) {
	r.requestTotal.inc(route, method, strconv.Itoa(status))
	r.requestDuration.observe(d.Seconds(), route, method)
}

func (r *Registry) CountBooking(kind, outcome string) {
	r.bookingTotal.inc(kind, outcome)
}

func (r *Registry) CountDatabaseError(sqlstate string) {
	r.dbErrors.inc(sqlstate)
}

func (r *Registry) CountRateLimited(route string) {
	r.rateLimited.inc(route)
}

func (r *Registry) SetBuildInfo(version string) {
	r.buildInfo.replace(1, version)
}

func (r *Registry) Render() string {
	var sb strings.Builder
	r.WriteTo(&sb)
	return sb.String()
}

func (r *Registry) WriteTo(sb *strings.Builder) {
	r.requestTotal.writeTo(sb)
	sb.WriteByte('\n')
	r.requestDuration.writeTo(sb)
	sb.WriteByte('\n')
	r.bookingTotal.writeTo(sb)
	sb.WriteByte('\n')
	r.dbErrors.writeTo(sb)
	sb.WriteByte('\n')
	r.rateLimited.writeTo(sb)
	sb.WriteByte('\n')
	r.buildInfo.writeTo(sb)
}
