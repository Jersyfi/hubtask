// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package mail_test

import (
	"context"
	"errors"
	"testing"

	health "github.com/Jersyfi/hubtask/core/port/health"
	port "github.com/Jersyfi/hubtask/core/port/mail"
	"github.com/Jersyfi/hubtask/infrastructure/mail"
	"github.com/Jersyfi/hubtask/infrastructure/resilience"
)

func TestTheProbeReadsTheBreakerRatherThanTheServer(t *testing.T) {
	breaker := resilience.NewBreaker(resilience.BreakerConfig{Dependency: "smtp"})
	probe := mail.NewProbe(breaker, true)

	if probe.Name() != "smtp" {
		t.Errorf("the probe is called %q", probe.Name())
	}
	if probe.Required() {
		t.Error("the mail server is required - then an SMTP outage would close the write path")
	}
	if got := probe.Check(context.Background()).Status; got != health.StatusOK {
		t.Errorf("a closed breaker reports %q", got)
	}

	// Enough failures to trip it. The threshold is the breaker's business; what matters here is
	// that the probe follows it rather than dialling.
	for range 100 {
		_ = breaker.Do(context.Background(), func(context.Context) error {
			return errors.New("refused")
		})
	}

	result := probe.Check(context.Background())
	if result.Status != health.StatusDown {
		t.Errorf("an open breaker reports %q", result.Status)
	}
	if result.ErrorCode != "dependency.unavailable" {
		t.Errorf("error code %q", result.ErrorCode)
	}
	// What a person cares about is being told things, not SMTP
	// (observability-reliability.md §7).
	if len(result.Impact) != 1 || result.Impact[0] != "notifications" {
		t.Errorf("impact %v, want [notifications]", result.Impact)
	}
}

// An installation that sends no email is not an installation that is broken. Reporting it as an
// outage would have an operator paging themselves over a decision they made.
func TestAnInstallationWithoutMailIsDisabledRatherThanDown(t *testing.T) {
	probe := mail.NewProbe(resilience.NewBreaker(resilience.BreakerConfig{Dependency: "smtp"}), false)

	result := probe.Check(context.Background())
	if result.Status != health.StatusDisabled {
		t.Errorf("status %q, want disabled", result.Status)
	}
	if len(result.Impact) != 0 {
		t.Errorf("a disabled dependency reports a degradation: %v", result.Impact)
	}
}

type recordingSender struct {
	calls int
	err   error
}

func (r *recordingSender) Send(context.Context, port.Message) error {
	r.calls++
	return r.err
}

// The breaker matters more here than anywhere else: a tenant with a busy morning has a queue full
// of notifications, and without it each would spend the dial timeout finding out what the one
// before it already knew.
func TestTheResilientSenderStopsCallingADeadServer(t *testing.T) {
	inner := &recordingSender{err: errors.New("refused")}
	breaker := resilience.NewBreaker(resilience.BreakerConfig{Dependency: "smtp"})
	bulkhead := resilience.NewBulkhead(resilience.BulkheadConfig{Name: "smtp", Capacity: 4})
	sender := mail.NewResilientSender(inner, breaker, bulkhead)

	for range 100 {
		_ = sender.Send(context.Background(), port.Message{To: "anna@test.invalid"})
	}

	if breaker.State() == resilience.BreakerClosed {
		t.Fatal("a hundred failures left the breaker closed")
	}
	if inner.calls >= 100 {
		t.Errorf("the sender was called %d times - the breaker let every one through", inner.calls)
	}
}

func TestTheResilientSenderPassesASuccessThrough(t *testing.T) {
	inner := &recordingSender{}
	sender := mail.NewResilientSender(inner,
		resilience.NewBreaker(resilience.BreakerConfig{Dependency: "smtp"}),
		resilience.NewBulkhead(resilience.BulkheadConfig{Name: "smtp", Capacity: 4}))

	if err := sender.Send(context.Background(), port.Message{To: "anna@test.invalid"}); err != nil {
		t.Fatalf("sending: %v", err)
	}
	if inner.calls != 1 {
		t.Errorf("the inner sender was called %d times", inner.calls)
	}
}
