package metrics

import (
	"sort"
	"strconv"
	"strings"
	"sync"
)

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
