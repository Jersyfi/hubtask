// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// workTrace is every call the request makes on a port, in order. Two requests whose traces are
// equal made the same calls - which is what T-02 needs of an answer that must not depend on whether
// an address holds an account, and what a stopwatch can only estimate.
type workTrace struct{ calls []string }

func (w *workTrace) note(format string, args ...any) {
	w.calls = append(w.calls, fmt.Sprintf(format, args...))
}

type tracedUnitOfWork struct {
	trace *workTrace
	inner persistence.UnitOfWork
}

func (u tracedUnitOfWork) Within(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	u.trace.note("within %s", scopeName(scope))
	return u.inner.Within(ctx, scope, fn)
}

func (u tracedUnitOfWork) WithinReadOnly(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	u.trace.note("read-only %s", scopeName(scope))
	return u.inner.WithinReadOnly(ctx, scope, fn)
}

func scopeName(scope persistence.Scope) string {
	if scope.Installation {
		return "installation"
	}
	return scope.TenantID.String()
}

type tracedAccounts struct {
	repository.PasswordAccounts
	trace *workTrace
}

func (a tracedAccounts) FindByEmailForPassword(ctx context.Context, email string) (repository.PasswordAccount, error) {
	a.trace.note("account lookup by address")
	return a.PasswordAccounts.FindByEmailForPassword(ctx, email)
}

func (a tracedAccounts) FindForPassword(ctx context.Context, accountID shared.ID) (repository.PasswordAccount, error) {
	a.trace.note("account read")
	return a.PasswordAccounts.FindForPassword(ctx, accountID)
}

type tracedSink struct {
	inner audit.Sink
	trace *workTrace
}

func (s tracedSink) Append(ctx context.Context, entry audit.Entry) error {
	s.trace.note("audit %s", entry.Action)
	return s.inner.Append(ctx, entry)
}

type tracedNotifier struct {
	notifierFake
	trace *workTrace
}

func (n *tracedNotifier) Enqueue(ctx context.Context, request queue.Request) (shared.ID, error) {
	keys := make([]string, 0, len(request.Payload))
	for key := range request.Payload {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	n.trace.note("enqueue %s dedupe=%t payload=%s", request.Kind, request.DedupeKey != "", strings.Join(keys, ","))
	return n.notifierFake.Enqueue(ctx, request)
}

// resetRequestFixture wires every port the request touches through one trace.
type resetRequestFixture struct {
	*passwordFixture
	trace    *workTrace
	notifier *tracedNotifier
}

func newResetRequestFixture() *resetRequestFixture {
	passwords := newPasswordFixture(now)
	trace := &workTrace{}
	f := &resetRequestFixture{
		passwordFixture: passwords, trace: trace,
		notifier: &tracedNotifier{trace: trace},
	}
	f.writer.UnitOfWork = tracedUnitOfWork{trace: trace, inner: f.writer.UnitOfWork}
	f.writer.Accounts = tracedAccounts{PasswordAccounts: passwords.accounts, trace: trace}
	f.writer.Session.Audit = tracedSink{inner: passwords.audit, trace: trace}
	return f
}

func (f *resetRequestFixture) forget() ForgetPassword {
	return ForgetPassword{
		Writer: f.writer, Notifier: f.notifier,
		Tenants: tenantDirectory{single: tenant}, Multi: true,
	}
}

func (f *resetRequestFixture) record() RecordResetRequest {
	return RecordResetRequest{Writer: f.writer, Notifier: f.notifier}
}

// runRequestJobs runs every request job queued so far, as the worker would, and answers what they
// queued in turn.
func (f *resetRequestFixture) runRequestJobs(t *testing.T) []queue.Request {
	t.Helper()
	pending := slices.Clone(f.notifier.queued)
	f.notifier.queued = nil
	for _, request := range pending {
		if request.Kind != queue.KindPasswordResetRequest {
			t.Fatalf("the request queued a %s job, want only %s", request.Kind, queue.KindPasswordResetRequest)
		}
		if err := f.record().Execute(t.Context(), request.TenantID, request.Payload); err != nil {
			t.Fatalf("the request job failed: %v", err)
		}
	}
	return f.notifier.queued
}

func resetForProviderOnly(f *resetRequestFixture) {
	held := f.accounts.byEmail["bert@example.org"]
	held.PasswordHash = secret.Secret{}
	f.accounts.byEmail["bert@example.org"] = held
	f.accounts.rows[account] = held
}

func disabled(f *resetRequestFixture) {
	held := f.accounts.byEmail["bert@example.org"]
	held.Account.Status = domain.AccountDisabled
	f.accounts.byEmail["bert@example.org"] = held
	f.accounts.rows[account] = held
}

func unheld(f *resetRequestFixture) { delete(f.accounts.byEmail, "bert@example.org") }

// UC-ID-04 check 1, T-02. The answer was always the same bytes; the work behind it was not - an
// address with an active account was recorded and its mail queued inside the request, and the time
// that took was the answer a probe read. Every address now makes the same calls, in the same order,
// with the same job: what differs happens in the job, after the answer.
func TestTheForgottenPasswordRequestDoesTheSameWorkForEveryAddress(t *testing.T) {
	addresses := map[string]func(*resetRequestFixture){
		"an account with a password": func(*resetRequestFixture) {},
		"a provider-only account":    resetForProviderOnly,
		"a disabled account":         disabled,
		"no account":                 unheld,
	}

	traces := map[string][]string{}
	for name, arrange := range addresses {
		fixture := newResetRequestFixture()
		arrange(fixture)

		if err := fixture.forget().Execute(t.Context(), ForgetPasswordCommand{
			Email: "Bert@Example.org", TenantSlug: "acme",
		}); err != nil {
			t.Fatalf("%s: answered %v, want the same silence", name, err)
		}
		traces[name] = fixture.trace.calls

		for _, call := range fixture.trace.calls {
			if strings.HasPrefix(call, "audit ") || strings.Contains(call, string(queue.KindPasswordResetEmail)) {
				t.Errorf("%s: the request itself did %q, which only an address with an account does", name, call)
			}
		}
	}

	want := traces["no account"]
	if !slices.ContainsFunc(want, func(call string) bool {
		return strings.HasPrefix(call, "enqueue "+string(queue.KindPasswordResetRequest))
	}) {
		t.Fatalf("an address with no account queued nothing (%q), so there is nothing behind the answer", want)
	}
	for name, got := range traces {
		if !slices.Equal(got, want) {
			t.Errorf("%s did different work from an address with no account:\n got %q\nwant %q", name, got, want)
		}
	}
}

// What the request queues is identifiers and a moment: the account or nobody, when it was asked,
// and the request that asked. Never the address (rule 10's reason for the queue table and every log
// line about a job).
func TestTheRequestJobCarriesIdentifiersOnly(t *testing.T) {
	for name, arrange := range map[string]func(*resetRequestFixture){
		"an account": func(*resetRequestFixture) {}, "no account": unheld,
	} {
		fixture := newResetRequestFixture()
		arrange(fixture)
		ctx := correlation.ContextWithRequestID(t.Context(), "req-forgot-1")

		if err := fixture.forget().Execute(ctx, ForgetPasswordCommand{
			Email: "bert@example.org", TenantSlug: "acme",
		}); err != nil {
			t.Fatalf("%s: refused: %v", name, err)
		}
		if len(fixture.notifier.queued) != 1 {
			t.Fatalf("%s: queued %v, want one request job", name, fixture.notifier.queued)
		}
		job := fixture.notifier.queued[0]
		if job.TenantID != tenant || job.DedupeKey != "" {
			t.Errorf("%s: the job is %+v, want the workspace's and no dedupe key", name, job)
		}
		if got := fmt.Sprint(job.Payload); strings.Contains(strings.ToLower(got), "example.org") {
			t.Errorf("%s: the payload carries the address: %s", name, got)
		}
		wantAccount := ""
		if name == "an account" {
			wantAccount = account.String()
		}
		if job.Payload["account_id"] != wantAccount || job.Payload["request_id"] != "req-forgot-1" ||
			job.Payload["requested_at"] != now.UTC().Format(time.RFC3339Nano) {
			t.Errorf("%s: the payload is %v", name, job.Payload)
		}
	}
}

// The job does what the request used to: a held, active account is recorded at the moment it was
// asked, under the request that asked, and gets its mail job; an address nobody holds and a disabled
// account get neither, so the trail never becomes a list of the addresses a guesser tried.
func TestTheRequestJobRecordsAndQueuesOnlyForAnActiveAccount(t *testing.T) {
	for name, arrange := range map[string]func(*resetRequestFixture){
		"an account with a password": func(*resetRequestFixture) {},
		"a provider-only account":    resetForProviderOnly,
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newResetRequestFixture()
			arrange(fixture)
			ctx := correlation.ContextWithRequestID(t.Context(), "req-forgot-2")
			if err := fixture.forget().Execute(ctx, ForgetPasswordCommand{
				Email: "bert@example.org", TenantSlug: "acme",
			}); err != nil {
				t.Fatalf("refused: %v", err)
			}

			mails := fixture.runRequestJobs(t)

			if len(mails) != 1 || mails[0].Kind != queue.KindPasswordResetEmail ||
				mails[0].Payload["account_id"] != account.String() || mails[0].DedupeKey != account.String() {
				t.Fatalf("queued %+v, want the account's one mail job", mails)
			}
			if _, leaked := mails[0].Payload["email"]; leaked {
				t.Error("the mail job carries an address")
			}
			entries := fixture.audit.entries
			if len(entries) != 1 || entries[0].Action != PasswordResetRequestedAction ||
				entries[0].TargetID != account || entries[0].Context.RequestID != "req-forgot-2" ||
				!entries[0].OccurredAt.Equal(now) {
				t.Errorf("the trail holds %+v, want one entry naming the account, the request and its moment", entries)
			}
		})
	}

	for name, arrange := range map[string]func(*resetRequestFixture){
		"no account": unheld, "a disabled account": disabled,
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newResetRequestFixture()
			arrange(fixture)
			if err := fixture.forget().Execute(t.Context(), ForgetPasswordCommand{
				Email: "bert@example.org", TenantSlug: "acme",
			}); err != nil {
				t.Fatalf("refused: %v", err)
			}

			if mails := fixture.runRequestJobs(t); len(mails) != 0 {
				t.Errorf("queued %+v, want nothing", mails)
			}
			if len(fixture.audit.entries) != 0 {
				t.Errorf("the trail holds %+v, want nothing", fixture.audit.entries)
			}
		})
	}
}

// An account that went away between the request and the job is finished business: nothing to
// record, nothing to send, and not a failure a retry would change.
func TestARequestJobForAnAccountGoneSinceDoesNothing(t *testing.T) {
	fixture := newResetRequestFixture()
	if err := fixture.forget().Execute(t.Context(), ForgetPasswordCommand{
		Email: "bert@example.org", TenantSlug: "acme",
	}); err != nil {
		t.Fatalf("refused: %v", err)
	}
	delete(fixture.accounts.rows, account)

	if mails := fixture.runRequestJobs(t); len(mails) != 0 || len(fixture.audit.entries) != 0 {
		t.Errorf("queued %v and recorded %v, want nothing", mails, fixture.audit.entries)
	}
}

// A payload that is not the request job's shape is a defect, not an address nobody holds.
func TestAMalformedRequestJobIsAnError(t *testing.T) {
	fixture := newResetRequestFixture()
	moment := now.UTC().Format(time.RFC3339Nano)
	for name, payload := range map[string]map[string]any{
		"empty":            {},
		"no moment":        {"account_id": account.String()},
		"a broken moment":  {"account_id": account.String(), "requested_at": "yesterday"},
		"a broken account": {"account_id": "bert", "requested_at": moment},
		"not a string":     {"account_id": 7, "requested_at": moment},
	} {
		if err := fixture.record().Execute(t.Context(), tenant, payload); err == nil {
			t.Errorf("%s: ran", name)
		}
	}
}

// The request never fails on what is behind it: a queue that refuses the job is logged and the
// answer is the same silence.
func TestARefusedRequestJobLeavesTheAnswerAlone(t *testing.T) {
	fixture := newResetRequestFixture()
	handler := fixture.forget()
	handler.Notifier = refusingNotifier{}

	if err := handler.Execute(t.Context(), ForgetPasswordCommand{
		Email: "bert@example.org", TenantSlug: "acme",
	}); err != nil {
		t.Fatalf("answered %v, want the same silence", err)
	}
}

type refusingNotifier struct{}

func (refusingNotifier) Enqueue(context.Context, queue.Request) (shared.ID, error) {
	return "", errors.New("queue unavailable")
}
