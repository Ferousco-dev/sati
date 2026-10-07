package metrics

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func render(t *testing.T, r *Registry) string {
	t.Helper()
	var b bytes.Buffer
	if err := r.WriteTo(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestCounterOutput(t *testing.T) {
	r := NewRegistry()
	c := r.Counter("jobs_total", "Jobs.", "kind", "result")
	c.Inc("email", "sent")
	c.Inc("email", "sent")
	c.Add(3, "email", "failed")
	c.Inc("only-one-label")

	out := render(t, r)
	for _, want := range []string{
		"# TYPE jobs_total counter",
		`jobs_total{kind="email",result="failed"} 3`,
		`jobs_total{kind="email",result="sent"} 2`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "only-one-label") {
		t.Fatal("a call with the wrong number of labels must be ignored")
	}
}

func TestHistogramOutput(t *testing.T) {
	r := NewRegistry()
	h := r.Histogram("latency_seconds", "Latency.", []float64{0.1, 1}, "route")
	h.Observe(0.05, "/a")
	h.Observe(0.5, "/a")
	h.Observe(5, "/a")

	out := render(t, r)
	for _, want := range []string{
		`latency_seconds_bucket{route="/a",le="0.1"} 1`,
		`latency_seconds_bucket{route="/a",le="1"} 2`,
		`latency_seconds_bucket{route="/a",le="+Inf"} 3`,
		`latency_seconds_count{route="/a"} 3`,
		`latency_seconds_sum{route="/a"} 5.55`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestLabelEscapingAndGauges(t *testing.T) {
	r := NewRegistry()
	r.Counter("x_total", "x", "v").Inc("a\"b\\c\nd")
	r.GaugeFunc("queue_depth", "Depth.", []string{"status"}, func(context.Context) ([]Sample, error) {
		return []Sample{{Labels: []string{"pending"}, Value: 4}}, nil
	})
	out := render(t, r)
	if !strings.Contains(out, `x_total{v="a\"b\\c\nd"} 1`) {
		t.Errorf("escaping: %s", out)
	}
	if !strings.Contains(out, `queue_depth{status="pending"} 4`) {
		t.Errorf("gauge: %s", out)
	}
}
