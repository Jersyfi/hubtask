// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package harness

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// Tick is the resolution of the timeline. Five seconds is fine enough to place an event - a
// rollout, the moment shedding engaged - inside a run, and coarse enough that the report stays
// readable.
const Tick = 5 * time.Second

// Latency is what a run felt like, not what it promised.
type Latency struct {
	Count int   `json:"count"`
	P50   int64 `json:"p50"`
	P95   int64 `json:"p95"`
	P99   int64 `json:"p99"`
	Max   int64 `json:"max"`
}

// Bucket is one interval of the timeline.
type Bucket struct {
	SecondsIn       int `json:"seconds_in"`
	OfferedRate     int `json:"offered_rate"`
	Requests        int `json:"requests"`
	NonSuccess      int `json:"non_success"`
	ServerErrors    int `json:"server_errors"`
	Shed            int `json:"shed"`
	TransportErrors int `json:"transport_errors"`
	// InteractiveP95 is the number RT-6 is about, per interval rather than over the run: an
	// average that holds while the middle three minutes were terrible is not evidence.
	InteractiveP95 int64 `json:"interactive_p95_ms"`

	interactive []int64
}

// Recorder collects what every worker sees.
//
// One mutex rather than per-worker counters merged at the end: the timeline needs each
// observation placed in time, and the contention of a few thousand requests a minute is not worth
// a lock-free design.
type Recorder struct {
	mu        sync.Mutex
	start     time.Time
	plan      Plan
	byStatus  map[string]int
	byClass   map[Class][]int64
	shed      map[Class]int
	transport int
	examples  []string
	buckets   map[int]*Bucket
	created   []string
}

func NewRecorder(start time.Time, plan Plan) *Recorder {
	return &Recorder{
		start:    start,
		plan:     plan,
		byStatus: map[string]int{},
		byClass:  map[Class][]int64{},
		shed:     map[Class]int{},
		buckets:  map[int]*Bucket{},
	}
}

// Observe records one answer. A status of zero with an error is a transport failure - a connection
// reset carries no status code at all, and a run that only counted 5xx would call that a success.
func (r *Recorder) Observe(class Class, status int, took time.Duration, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	bucket := r.bucketAt(now)
	bucket.Requests++

	if err != nil {
		r.transport++
		bucket.TransportErrors++
		bucket.NonSuccess++
		if len(r.examples) < 5 {
			r.examples = append(r.examples, err.Error())
		}
		return
	}

	r.byStatus[fmt.Sprint(status)]++
	if status >= 300 {
		bucket.NonSuccess++
	}
	if status >= 500 {
		bucket.ServerErrors++
	}
	// A shed call is a 503 the harness asked for, so it is counted apart from the server errors
	// it would otherwise be filed under - and its latency is left out of the class percentile.
	// A refusal is fast by construction, and letting thousands of them into the sample would
	// make the P95 fall as the overload got worse.
	if status == 503 {
		r.shed[class]++
		bucket.Shed++
		return
	}

	r.byClass[class] = append(r.byClass[class], took.Milliseconds())
	if class == ClassInteractive {
		bucket.interactive = append(bucket.interactive, took.Milliseconds())
	}
}

// Remember keeps an identifier the installation said it created, so that the run can go looking
// for all of them afterwards.
func (r *Recorder) Remember(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.created = append(r.created, id)
}

// Created is what was written, in the order it was written.
func (r *Recorder) Created() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.created...)
}

func (r *Recorder) bucketAt(now time.Time) *Bucket {
	resolution := int(Tick.Seconds())
	elapsed := now.Sub(r.start)
	seconds := int(elapsed.Seconds()) / resolution * resolution
	bucket, known := r.buckets[seconds]
	if !known {
		bucket = &Bucket{SecondsIn: seconds, OfferedRate: r.plan.RateAt(time.Duration(seconds) * time.Second)}
		r.buckets[seconds] = bucket
	}
	return bucket
}

// LastClosed is the interval that has just ended, and it never creates one. The progress ticker
// fires on the boundary, so the interval current at that moment is always the one nobody has
// written to yet - reporting it printed a run of zeroes through a run that was working.
func (r *Recorder) LastClosed(now time.Time) Bucket {
	r.mu.Lock()
	defer r.mu.Unlock()

	resolution := int(Tick.Seconds())
	seconds := int(now.Sub(r.start).Seconds())/resolution*resolution - resolution
	if bucket, known := r.buckets[seconds]; known {
		closed := *bucket
		closed.InteractiveP95 = Percentiles(bucket.interactive).P95
		return closed
	}
	return Bucket{SecondsIn: seconds}
}

// Window is a slice of a run, measured from its start: from is inclusive, to is exclusive. It
// exists as a type so that a caller narrowing one - the last half of a stage rather than the whole
// of it - says so in one place instead of arithmetic at the call site.
type Window struct {
	From time.Duration
	To   time.Duration
}

// Window is the latency of one class over a slice of the run, pooled from the raw samples rather
// than averaged out of the per-interval percentiles. A percentile of percentiles is not a
// percentile of anything, and the stage comparison RT-6 rests on would be built out of one.
//
// from is inclusive and to is exclusive, both measured from the start of the run. It exists so a
// ramp can be read stage by stage: the claim worth making is not "the P95 held over the run" but
// "the P95 under overload was within a factor of the P95 before it".
func (r *Recorder) Window(class Class, from, to time.Duration) Latency {
	r.mu.Lock()
	defer r.mu.Unlock()

	var pooled []int64
	for seconds, bucket := range r.buckets {
		at := time.Duration(seconds) * time.Second
		if at < from || at >= to {
			continue
		}
		if class == ClassInteractive {
			pooled = append(pooled, bucket.interactive...)
		}
	}
	return Percentiles(pooled)
}

// Throughput is how many calls the installation actually answered per second over a window,
// counting neither the refusals nor the connections that never got one.
//
// It is the capacity figure and it only means something once the offered rate is past what the
// process can serve: below saturation this returns the offered rate, because that is what was
// asked of it. Which is why the figure H-11 records comes from an overload stage and not from a
// steady one - a run that kept up measured the client.
func (r *Recorder) Throughput(from, to time.Duration) float64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	seconds := (to - from).Seconds()
	if seconds <= 0 {
		return 0
	}
	answered := 0
	for at, bucket := range r.buckets {
		elapsed := time.Duration(at) * time.Second
		if elapsed < from || elapsed >= to {
			continue
		}
		answered += bucket.Requests - bucket.Shed - bucket.TransportErrors
	}
	return float64(answered) / seconds
}

// Shed is how many calls of a class were refused. Read while the run is still going, which is what
// lets a test say the mechanism engaged before it says anything about latency.
func (r *Recorder) Shed(class Class) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.shed[class]
}

// Summary is the finding of one run, and it is JSON because it ends up quoted in an evidence
// document and compared against a stored baseline.
type Summary struct {
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
	// OfferedPeak is the plan's ceiling. Recorded beside the achieved rate, because a run that
	// offered 400 a second and achieved 120 is a capacity finding rather than a failed run.
	OfferedPeak     int                `json:"offered_peak_per_second"`
	Requests        int                `json:"requests"`
	ByStatus        map[string]int     `json:"by_status"`
	Shed            map[string]int     `json:"shed_by_class"`
	TransportErrors int                `json:"transport_errors"`
	ErrorExamples   []string           `json:"transport_error_examples,omitempty"`
	Latency         map[string]Latency `json:"latency_ms_by_class"`
	Timeline        []Bucket           `json:"timeline"`
}

// Summarise closes the run. It takes no lock beyond its own: every worker is finished by the time
// a caller asks for this.
func (r *Recorder) Summarise(ended time.Time) Summary {
	r.mu.Lock()
	defer r.mu.Unlock()

	requests := r.transport
	for _, n := range r.byStatus {
		requests += n
	}

	timeline := make([]Bucket, 0, len(r.buckets))
	for _, bucket := range r.buckets {
		closed := *bucket
		closed.InteractiveP95 = Percentiles(bucket.interactive).P95
		closed.interactive = nil
		timeline = append(timeline, closed)
	}
	sort.Slice(timeline, func(i, j int) bool { return timeline[i].SecondsIn < timeline[j].SecondsIn })

	latency := map[string]Latency{}
	for class, samples := range r.byClass {
		latency[string(class)] = Percentiles(samples)
	}
	shed := map[string]int{}
	for class, n := range r.shed {
		shed[string(class)] = n
	}

	return Summary{
		StartedAt:       r.start.UTC(),
		EndedAt:         ended.UTC(),
		OfferedPeak:     r.plan.Peak(),
		Requests:        requests,
		ByStatus:        r.byStatus,
		Shed:            shed,
		TransportErrors: r.transport,
		ErrorExamples:   r.examples,
		Latency:         latency,
		Timeline:        timeline,
	}
}

// ServerErrors is how many answers were a 5xx that the harness did not ask for. A shed 503 is
// excluded: it is the mechanism working, not the installation failing.
func (s Summary) ServerErrors() int {
	errors := 0
	for status, n := range s.ByStatus {
		if len(status) == 3 && status[0] == '5' {
			errors += n
		}
	}
	for _, n := range s.Shed {
		errors -= n
	}
	return errors
}

// Percentiles sorts a copy rather than the caller's slice: the samples belong to a recorder that
// is still holding them, and sorting them in place would reorder a live buffer.
func Percentiles(samples []int64) Latency {
	if len(samples) == 0 {
		return Latency{}
	}
	sorted := append([]int64(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	at := func(q float64) int64 {
		return sorted[int(float64(len(sorted)-1)*q)]
	}
	return Latency{
		Count: len(sorted),
		P50:   at(0.50),
		P95:   at(0.95),
		P99:   at(0.99),
		Max:   sorted[len(sorted)-1],
	}
}
