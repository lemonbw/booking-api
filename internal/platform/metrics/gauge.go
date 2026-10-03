package metrics

import (
	"sort"
	"strings"
	"sync"
)

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

func (g *gauge) replace(value float64, values ...string) {
	g.mu.Lock()
	clear(g.values)
	g.values[encodeLabels(g.labels, values)] = value
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
