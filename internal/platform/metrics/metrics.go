package metrics

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
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
	r.buildInfo.set(1, version)
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

type counterVec struct {
	name   string
	help   string
	labels []string

	mu     sync.Mutex
	values map[string]float64
}

func newCounterVec(name, help string, labels []string) *counterVec {
	return &counterVec{name: name, help: help, labels: labels, values: make(map[string]float64)}
}

func (c *counterVec) inc(values ...string) {
	key := encodeLabels(c.labels, values)

	c.mu.Lock()
	c.values[key]++
	c.mu.Unlock()
}

func (c *counterVec) writeTo(sb *strings.Builder) {
	c.mu.Lock()
	keys := make([]string, 0, len(c.values))
	for k := range c.values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	values := make(map[string]float64, len(c.values))
	for k, v := range c.values {
		values[k] = v
	}
	c.mu.Unlock()

	writeHeader(sb, c.name, c.help, "counter")
	for _, key := range keys {
		writeValue(sb, c.name, parseLabelKey(key), values[key])
	}
}

type histogramVec struct {
	name    string
	help    string
	labels  []string
	buckets []float64

	mu     sync.Mutex
	series map[string]*histogramSeries
}
type histogramSeries struct {
	counts []uint64
	sum    float64
	total  uint64
}

func newHistogramVec(name, help string, labels []string, buckets []float64) *histogramVec {
	return &histogramVec{name: name, help: help, labels: labels, buckets: buckets, series: make(map[string]*histogramSeries)}
}

func (h *histogramVec) observe(seconds float64, values ...string) {
	key := encodeLabels(h.labels, values)

	h.mu.Lock()
	defer h.mu.Unlock()

	s, ok := h.series[key]
	if !ok {
		s = &histogramSeries{counts: make([]uint64, len(h.buckets))}
		h.series[key] = s
	}
	for i, upper := range h.buckets {
		if seconds <= upper {
			s.counts[i]++
		}
	}
	s.sum += seconds
	s.total++
}

func (h *histogramVec) writeTo(sb *strings.Builder) {
	h.mu.Lock()
	keys := make([]string, 0, len(h.series))
	for k := range h.series {
		keys = append(keys, k)
	}
	snapshot := make(map[string]histogramSeries, len(h.series))
	for k, v := range h.series {
		snapshot[k] = histogramSeries{counts: append([]uint64(nil), v.counts...), sum: v.sum, total: v.total}
	}
	h.mu.Unlock()

	sort.Strings(keys)

	writeHeader(sb, h.name, h.help, "histogram")
	for _, key := range keys {
		s := snapshot[key]
		labels := parseLabelKey(key)
		for i, upper := range h.buckets {
			writeSample(sb, h.name, labels, "le", strconv.FormatFloat(upper, 'f', -1, 64), s.counts[i])
		}
		writeSample(sb, h.name, labels, "le", "+Inf", s.total)
		sb.WriteString(h.name)
		sb.WriteString("_sum")
		writeLabels(sb, labels)
		sb.WriteByte(' ')
		sb.WriteString(strconv.FormatFloat(s.sum, 'f', -1, 64))
		sb.WriteByte('\n')
		writeCount(sb, h.name, labels, s.total)
	}
}

func writeSample(sb *strings.Builder, name string, labels map[string]string, labelName, labelValue string, value uint64) {
	sb.WriteString(name)
	sb.WriteString("_bucket")
	writeLabels(sb, mergeLabels(labels, labelName, labelValue))
	sb.WriteByte(' ')
	sb.WriteString(strconv.FormatUint(value, 10))
	sb.WriteByte('\n')
}

func writeCount(sb *strings.Builder, name string, labels map[string]string, value uint64) {
	sb.WriteString(name)
	sb.WriteString("_count")
	writeLabels(sb, labels)
	sb.WriteByte(' ')
	sb.WriteString(strconv.FormatUint(value, 10))
	sb.WriteByte('\n')
}

func writeValue(sb *strings.Builder, name string, labels map[string]string, value float64) {
	sb.WriteString(name)
	writeLabels(sb, labels)
	sb.WriteByte(' ')
	sb.WriteString(strconv.FormatFloat(value, 'f', -1, 64))
	sb.WriteByte('\n')
}

func writeLabels(sb *strings.Builder, labels map[string]string) {
	if len(labels) == 0 {
		return
	}
	sb.WriteByte('{')
	sb.WriteString(joinLabelNames(labels))
	sb.WriteByte('}')
}

type gauge struct {
	name   string
	help   string
	labels []string

	mu     sync.Mutex
	values map[string]float64
}

func newGauge(name, help string, labels []string) *gauge {
	return &gauge{name: name, help: help, labels: labels, values: make(map[string]float64)}
}

func (g *gauge) set(value float64, values ...string) {
	key := encodeLabels(g.labels, values)

	g.mu.Lock()
	g.values[key] = value
	g.mu.Unlock()
}

func (g *gauge) writeTo(sb *strings.Builder) {
	g.mu.Lock()
	keys := make([]string, 0, len(g.values))
	for k := range g.values {
		keys = append(keys, k)
	}
	values := make(map[string]float64, len(g.values))
	for k, v := range g.values {
		values[k] = v
	}
	g.mu.Unlock()

	sort.Strings(keys)

	writeHeader(sb, g.name, g.help, "gauge")
	for _, key := range keys {
		writeValue(sb, g.name, parseLabelKey(key), values[key])
	}
}

func writeHeader(sb *strings.Builder, name, help, metricType string) {
	sb.WriteString("# HELP ")
	sb.WriteString(name)
	sb.WriteByte(' ')
	sb.WriteString(help)
	sb.WriteByte('\n')
	sb.WriteString("# TYPE ")
	sb.WriteString(name)
	sb.WriteByte(' ')
	sb.WriteString(metricType)
	sb.WriteByte('\n')
}

func encodeLabels(names, values []string) string {
	var sb strings.Builder
	for i, name := range names {
		if i > 0 {
			sb.WriteByte(0)
		}
		sb.WriteString(name)
		sb.WriteByte(0)
		if i < len(values) {
			sb.WriteString(values[i])
		}
	}
	return sb.String()
}

func parseLabelKey(key string) map[string]string {
	out := make(map[string]string)
	parts := strings.Split(key, "\x00")
	for i := 0; i+1 < len(parts); i += 2 {
		if parts[i] == "" {
			continue
		}
		out[parts[i]] = parts[i+1]
	}
	return out
}

func joinLabelNames(labels map[string]string) string {
	names := make([]string, 0, len(labels))
	for name := range labels {
		names = append(names, name)
	}
	sort.Strings(names)

	pairs := make([]string, 0, len(names))
	for _, name := range names {
		pairs = append(pairs, fmt.Sprintf("%s=%q", name, escapeLabelValue(labels[name])))
	}
	return strings.Join(pairs, ",")
}

func mergeLabels(labels map[string]string, name, value string) map[string]string {
	merged := make(map[string]string, len(labels)+1)
	for key, val := range labels {
		merged[key] = val
	}
	merged[name] = value
	return merged
}

func escapeLabelValue(v string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(v)
}
