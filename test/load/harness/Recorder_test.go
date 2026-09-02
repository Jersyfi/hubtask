// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package harness

import (
	"errors"
	"testing"
	"time"
)

func TestPercentilesAreTakenFromTheWholeSample(t *testing.T) {
	samples := make([]int64, 100)
	for i := range samples {
		samples[i] = int64(i + 1)
	}

	got := Percentiles(samples)
	if got.Count != 100 || got.P50 != 50 || got.P95 != 95 || got.P99 != 99 || got.Max != 100 {
		t.Errorf("percentiles = %+v", got)
	}
}

// The caller's slice belongs to a recorder that is still appending to it. Sorting it in place
// would reorder a live buffer, and the symptom would be a timeline that drifts rather than a
// crash.
func TestPercentilesDoNotReorderTheCallersSample(t *testing.T) {
	samples := []int64{9, 3, 7}
	Percentiles(samples)
	if samples[0] != 9 || samples[1] != 3 || samples[2] != 7 {
		t.Errorf("the sample was sorted in place: %v", samples)
	}
}

// The distinction the whole of RT-6 turns on: a refused deferrable call is the mechanism working,
// and it must not be counted as a server error and must not enter the latency sample. A refusal is
// fast by construction, so letting thousands of them in would make the P95 *fall* as the overload
// got worse.
func TestAShedCallIsNeitherAFailureNorAFastAnswer(t *testing.T) {
	recorder := NewRecorder(time.Now(), FlatPlan(10, time.Minute))

	for range 10 {
		recorder.Observe(ClassDeferrable, 503, time.Millisecond, nil)
	}
	recorder.Observe(ClassInteractive, 200, 100*time.Millisecond, nil)

	summary := recorder.Summarise(time.Now())
	if summary.Shed[string(ClassDeferrable)] != 10 {
		t.Errorf("shed = %v, want 10 deferrable", summary.Shed)
	}
	if summary.ServerErrors() != 0 {
		t.Errorf("server errors = %d, want 0 - a shed 503 is not a failure", summary.ServerErrors())
	}
	if _, sampled := summary.Latency[string(ClassDeferrable)]; sampled {
		t.Error("the refusals entered the deferrable latency sample")
	}
	if summary.Latency[string(ClassInteractive)].P95 != 100 {
		t.Errorf("interactive P95 = %d, want 100", summary.Latency[string(ClassInteractive)].P95)
	}
}

// A 503 the installation produced for its own reasons is a failure and has to stay one. The
// harness cannot tell the two apart by status alone, which is why the class matters: what the
// summary subtracts is exactly what it counted as shed.
func TestAServerErrorIsStillAFailure(t *testing.T) {
	recorder := NewRecorder(time.Now(), FlatPlan(10, time.Minute))

	recorder.Observe(ClassInteractive, 500, time.Millisecond, nil)
	recorder.Observe(ClassDeferrable, 503, time.Millisecond, nil)

	summary := recorder.Summarise(time.Now())
	if summary.ServerErrors() != 1 {
		t.Errorf("server errors = %d, want 1", summary.ServerErrors())
	}
}

// A connection reset carries no status code at all. A run that only counted 5xx would file it as
// a success, which is the exact failure an overloaded or restarting process produces.
func TestATransportFailureIsCountedAndKeptVerbatim(t *testing.T) {
	recorder := NewRecorder(time.Now(), FlatPlan(10, time.Minute))

	recorder.Observe(ClassInteractive, 0, time.Second, errors.New("connection reset by peer"))

	summary := recorder.Summarise(time.Now())
	if summary.TransportErrors != 1 {
		t.Errorf("transport errors = %d, want 1", summary.TransportErrors)
	}
	if len(summary.ErrorExamples) != 1 {
		t.Fatalf("examples = %v", summary.ErrorExamples)
	}
	if summary.Requests != 1 {
		t.Errorf("requests = %d, want 1 - a failed request is still a request", summary.Requests)
	}
}

// The timeline is what turns "P95 held over the run" into "P95 held while shedding was engaged".
// Each interval carries the rate that was offered in it, so a ramp can be read back off the
// report without the plan beside it.
func TestTheTimelineCarriesTheOfferedRateOfEachInterval(t *testing.T) {
	start := time.Now().Add(-3 * Tick)
	recorder := NewRecorder(start, Plan{
		{PerSecond: 10, For: 2 * Tick},
		{PerSecond: 90, For: 2 * Tick},
	})

	recorder.Observe(ClassInteractive, 200, 40*time.Millisecond, nil)

	summary := recorder.Summarise(time.Now())
	if len(summary.Timeline) != 1 {
		t.Fatalf("timeline = %+v", summary.Timeline)
	}
	interval := summary.Timeline[0]
	if interval.SecondsIn != 3*int(Tick.Seconds()) {
		t.Errorf("the observation landed in interval %d", interval.SecondsIn)
	}
	if interval.OfferedRate != 90 {
		t.Errorf("offered rate = %d, want the second stage's 90", interval.OfferedRate)
	}
	if interval.InteractiveP95 != 40 {
		t.Errorf("interval P95 = %d, want 40", interval.InteractiveP95)
	}
}
