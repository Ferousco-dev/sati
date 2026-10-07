// Package metrics is a small Prometheus-compatible registry. It exists so the
// API can expose metrics without an extra dependency.
package metrics

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
)

type Registry struct {
	mu       sync.Mutex
	counters []*CounterVec
	hists    []*HistogramVec
	gauges   []*gaugeFunc
}

func NewRegistry() *Registry { return &Registry{} }

type CounterVec struct {
	name, help string
	labels     []string
	mu         sync.Mutex
	values     map[string]float64
}

type HistogramVec struct {
	name, help string
	labels     []string
	buckets    []float64
	mu         sync.Mutex
	series     map[string]*histogram
}

type histogram struct {
	counts []uint64
	sum    float64
	total  uint64
}

type Sample struct {
	Labels []string
	Value  float64
}

type gaugeFunc struct {
	name, help string
	labels     []string
	collect    func(ctx context.Context) ([]Sample, error)
}

func (r *Registry) Counter(name, help string, labels ...string) *CounterVec {
	c := &CounterVec{name: name, help: help, labels: labels, values: map[string]float64{}}
	r.mu.Lock()
	r.counters = append(r.counters, c)
	r.mu.Unlock()
	return c
}

func (r *Registry) Histogram(name, help string, buckets []float64, labels ...string) *HistogramVec {
	h := &HistogramVec{name: name, help: help, labels: labels, buckets: buckets, series: map[string]*histogram{}}
	r.mu.Lock()
	r.hists = append(r.hists, h)
	r.mu.Unlock()
	return h
}

// GaugeFunc registers values that are read fresh on every scrape.
func (r *Registry) GaugeFunc(name, help string, labels []string, collect func(ctx context.Context) ([]Sample, error)) {
	r.mu.Lock()
	r.gauges = append(r.gauges, &gaugeFunc{name: name, help: help, labels: labels, collect: collect})
	r.mu.Unlock()
}

const sep = "\x00"

func (c *CounterVec) Inc(labelValues ...string) { c.Add(1, labelValues...) }

func (c *CounterVec) Add(v float64, labelValues ...string) {
	if len(labelValues) != len(c.labels) {
		return
	}
	key := strings.Join(labelValues, sep)
	c.mu.Lock()
	c.values[key] += v
	c.mu.Unlock()
}

func (h *HistogramVec) Observe(v float64, labelValues ...string) {
	if len(labelValues) != len(h.labels) {
		return
	}
	key := strings.Join(labelValues, sep)
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.series[key]
	if s == nil {
		s = &histogram{counts: make([]uint64, len(h.buckets))}
		h.series[key] = s
	}
	for i, upper := range h.buckets {
		if v <= upper {
			s.counts[i]++
		}
	}
	s.sum += v
	s.total++
}

func labelString(names []string, values []string, extra ...string) string {
	var parts []string
	for i, n := range names {
		parts = append(parts, fmt.Sprintf(`%s="%s"`, n, escape(values[i])))
	}
	parts = append(parts, extra...)
	if len(parts) == 0 {
		return ""
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func escape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// WriteTo renders every metric in the Prometheus text format.
func (r *Registry) WriteTo(ctx context.Context, w io.Writer) error {
	r.mu.Lock()
	counters, hists, gauges := append([]*CounterVec(nil), r.counters...), append([]*HistogramVec(nil), r.hists...), append([]*gaugeFunc(nil), r.gauges...)
	r.mu.Unlock()

	for _, c := range counters {
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n", c.name, c.help, c.name)
		c.mu.Lock()
		for _, key := range sortedKeys(c.values) {
			fmt.Fprintf(w, "%s%s %g\n", c.name, labelString(c.labels, strings.Split(key, sep)), c.values[key])
		}
		c.mu.Unlock()
	}

	for _, h := range hists {
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s histogram\n", h.name, h.help, h.name)
		h.mu.Lock()
		for _, key := range sortedKeys(h.series) {
			values, s := strings.Split(key, sep), h.series[key]
			for i, upper := range h.buckets {
				fmt.Fprintf(w, "%s_bucket%s %d\n", h.name, labelString(h.labels, values, fmt.Sprintf(`le="%g"`, upper)), s.counts[i])
			}
			fmt.Fprintf(w, "%s_bucket%s %d\n", h.name, labelString(h.labels, values, `le="+Inf"`), s.total)
			fmt.Fprintf(w, "%s_sum%s %g\n%s_count%s %d\n", h.name, labelString(h.labels, values), s.sum, h.name, labelString(h.labels, values), s.total)
		}
		h.mu.Unlock()
	}

	for _, g := range gauges {
		samples, err := g.collect(ctx)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n", g.name, g.help, g.name)
		for _, s := range samples {
			fmt.Fprintf(w, "%s%s %g\n", g.name, labelString(g.labels, s.Labels), s.Value)
		}
	}
	return nil
}
