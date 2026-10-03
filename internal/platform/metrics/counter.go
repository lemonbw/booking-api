package metrics

import (
	"sort"
	"strings"
	"sync"
)

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
