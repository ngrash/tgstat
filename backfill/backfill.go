// Package backfill provides mechanisms to generate metrics from historical data.
// Metrics are recorded at a specific time and can be written to an io.Writer
// with a consistent resolution, as is required by many time series databases.
package backfill

import (
	"fmt"
	"io"
	"time"
)

// ErrNoRecords is returned if Metrics.Write is called before
// any metrics were recorded.
var ErrNoRecords = fmt.Errorf("no records")

// recorder defines the interface for recording metrics.
type recorder interface {
	Inc(name string, value uint64, at time.Time)
	Write(w io.Writer, resolution time.Duration) error
}

// Metrics is a collection of metrics that share the same labels.
type Metrics struct {
	labels labels
	rec    recorder
}

// NewMetrics creates a new Metrics instance.
func NewMetrics() *Metrics {
	return newMetricsWithRecorder(newLinkedListRecorder())
}

// newMetricsWithRecorder creates a new Metrics instance with the given recorder.
// Used for testing.
func newMetricsWithRecorder(rec recorder) *Metrics {
	return &Metrics{
		labels: labels{},
		rec:    rec,
	}
}

// With returns a copy of the Metrics with an additional label appended.
func (m *Metrics) With(key, value string) *Metrics {
	return &Metrics{
		labels: m.labels.with(key, value),
		rec:    m.rec,
	}
}

// Metric returns a Metric instance that can be used to record values.
// It returned Metric inherits the labels from the Metrics instance.
func (m *Metrics) Metric(name string) *Metric {
	return &Metric{
		name:   name,
		labels: m.labels,
		rec:    m.rec,
	}
}

// Write the Metrics to the given io.Writer with the given resolution.
func (m *Metrics) Write(w io.Writer, resolution time.Duration) error {
	return m.rec.Write(w, resolution)
}

// Metric represents a single metric that can be recorded.
type Metric struct {
	name   string
	labels labels
	rec    recorder
}

// Inc records an increment of the metric by the given value at the given time.
func (m *Metric) Inc(value uint64, at time.Time) {
	s := fmt.Sprintf("%s{%s}", m.name, m.labels.String())
	m.rec.Inc(s, value, at)
}

// With returns a copy of the Metric with an additional label appended.
func (m *Metric) With(key, value string) *Metric {
	return &Metric{
		name:   m.name,
		labels: m.labels.with(key, value),
		rec:    m.rec,
	}
}

// labels is a slice of label instances.
type labels []label

// with returns a copy of labels with an additional label appended.
func (l labels) with(key, value string) labels {
	newLabels := make(labels, len(l)+1)
	copy(newLabels, l)
	newLabels[len(l)] = label{key, value}
	return newLabels
}

// String returns a string representation of the labels.
// Can be used to construct a metric name. Label values are
// quoted as Go string literals.
func (l labels) String() string {
	var s string
	for _, label := range l {
		s += fmt.Sprintf("%s=%#v,", label.key, label.value)
	}
	return s[:len(s)-1]
}

// label is a key-value pair that adds context to a Metric.
type label struct {
	key   string
	value string
}

// record is a single data point in time.
// It is used by linkedListRecorder to store the data points in a linked list.
type record struct {
	value uint64
	at    time.Time
	next  *record
}

// forward return the record that represents the value at the given time and
// a boolean indicating if the record has followup records.
func (r *record) forward(to time.Time) (*record, bool) {
	//fmt.Println("move", r.at.Unix(), "to", to.Unix())
	if r.at.After(to) {
		// not yet started
		return nil, true // followup is r itself
	}
	if r.next == nil {
		return r, false // no followup
	}
	if r.next.at.After(to) {
		return r, true // followup is next
	}
	return r.next.forward(to)
}

// linkedListRecorder implements the recorder interface using a linked list.
type linkedListRecorder struct {
	first   map[string]*record
	current map[string]*record
}

func newLinkedListRecorder() *linkedListRecorder {
	return &linkedListRecorder{
		first:   make(map[string]*record),
		current: make(map[string]*record),
	}
}

func (r *linkedListRecorder) Inc(name string, value uint64, at time.Time) {
	if current, ok := r.current[name]; ok {
		if current.at.After(at) {
			fmt.Printf("backfill: %s: ignoring record at %d, current is at %d\n", name, at.Unix(), current.at.Unix())
			return
		}
		next := &record{current.value + value, at, nil}
		current.next = next
		r.current[name] = next
	} else { // first time
		next := &record{value, at, nil}
		r.first[name] = next
		r.current[name] = next
	}
}

func (r *linkedListRecorder) Write(w io.Writer, resolution time.Duration) error {
	// First record determines the start time.
	var start *time.Time
	for _, f := range r.first {
		if start == nil || f.at.Before(*start) {
			start = &f.at
		}
	}
	if start == nil {
		return ErrNoRecords
	}

	current := map[string]*record{}
	for name, r := range r.first {
		current[name] = r
	}

	// Walk through time in resolution steps.
	for now := *start; ; now = now.Add(resolution) {
		//fmt.Println("step", now.Unix())

		// Advance all metrics to the record at the current time.
		// If the metrics has no record that is active at the current time,
		// it is skipped.
		// If a metric has a record at the current time and future records,
		// it is considered active and written to the output.
		// If a metric has a record at the current time but no followup record,
		// it is considered inactive but still written with the last value.
		// When no more metrics are active, the loop ends.
		var hasActiveMetrics bool
		for name, r := range current {
			next, hasMore := r.forward(now)
			if next == nil {
				// not yet started
				continue
			}
			if hasMore {
				hasActiveMetrics = true
				// Move to next record.
				current[name] = next
			}

			// Write the record.
			_, err := fmt.Fprintf(w, "%s %d %d\n", name, next.value, now.Unix())
			if err != nil {
				return err
			}
		}
		// All metrics are inactive. We are done.
		if !hasActiveMetrics {
			break
		}
	}
	return nil
}
