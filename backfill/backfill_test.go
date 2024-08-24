package backfill

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

type labelTestRecorder struct {
	names []string
}

func (r *labelTestRecorder) Inc(name string, _ uint64, _ time.Time) {
	r.names = append(r.names, name)
}

func (r *labelTestRecorder) Render(_ io.Writer, _ time.Duration) {}

func TestMetrics(t *testing.T) {
	tr := &labelTestRecorder{}
	m := newMetricsWithRecorder(tr)
	fooMetrics := m.With("x", "foo")
	barMetrics := m.With("x", "bar")

	fooMetrics.Metric("qux").Inc(1, time.Time{})
	fooMetrics.With("y", "baz").Metric("qux").Inc(1, time.Time{})
	barMetrics.Metric("qux").Inc(1, time.Time{})
	barMetrics.Metric("zot").Inc(1, time.Time{})

	want := []string{
		"qux{x=\"foo\"}",
		"qux{x=\"foo\",y=\"baz\"}",
		"qux{x=\"bar\"}",
		"zot{x=\"bar\"}",
	}
	if diff := cmp.Diff(want, tr.names); diff != "" {
		t.Errorf("diff -want +got:\n%s", diff)
	}
}

func TestLinkedListRecorder(t *testing.T) {
	start := time.Unix(1724512000, 0)

	r := newLinkedListRecorder()
	r.Inc("foo", 1, start.Add(00*time.Second)) // 1
	r.Inc("foo", 1, start.Add(10*time.Second)) // 2

	// Record in between resolution steps.
	// It should increase the value but not be rendered.
	r.Inc("foo", 1, start.Add(15*time.Second)) // 3

	r.Inc("foo", 1, start.Add(20*time.Second)) // 4

	r.Inc("foo", 1, start.Add(33*time.Second)) // 5

	var b strings.Builder
	r.Render(&b, 10*time.Second)

	got := b.String()
	want := "foo 1 1724512000\n"
	want += "foo 2 1724512010\n"
	want += "foo 4 1724512020\n"
	want += "foo 4 1724512030\n"
	want += "foo 5 1724512040\n"
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("diff -want +got:\n%s", diff)
	}
}
