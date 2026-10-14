// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/health"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

type listedWorkspaces []adminrepo.TenantRecord

func (l listedWorkspaces) List(context.Context) ([]adminrepo.TenantRecord, error) { return l, nil }

// coverageByWorkspace answers each workspace's own count, read from the scope the unit of work
// put into the context - which is how row level security bounds the real statement.
type coverageByWorkspace struct {
	targetStore
	mu  sync.Mutex
	by  map[shared.ID]repository.Coverage
	err error
}

type scopeKey struct{}

func (c *coverageByWorkspace) Coverage(ctx context.Context) (repository.Coverage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	scope, _ := ctx.Value(scopeKey{}).(persistence.Scope)
	return c.by[scope.TenantID], c.err
}

type scopedWork struct{}

func (scopedWork) Within(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	return fn(context.WithValue(ctx, scopeKey{}, scope))
}

func (w scopedWork) WithinReadOnly(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	return w.Within(ctx, scope, fn)
}

type stillClock struct{ now time.Time }

func (c stillClock) Now() time.Time { return c.now }

// settledTargets waits for the background count the last report started.
func settledTargets(t *testing.T, w *TargetWarnings) {
	t.Helper()
	for range 200 {
		w.mu.Lock()
		running := w.running
		w.mu.Unlock()
		if !running {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the background count never finished")
}

func codesOf(warnings []health.Warning) map[string]health.Warning {
	codes := map[string]health.Warning{}
	for _, warning := range warnings {
		codes[warning.Code] = warning
	}
	return codes
}

// backup-restore.md §10, P-11: the installation is told when nothing, or only one place, holds its
// backups, and when some of them are stored unencrypted - counted across every workspace.
func TestTheBackupWarningsSayWhatTheTargetsAreAcrossTheWorkspaces(t *testing.T) {
	first := shared.MustParseID("018f2a1b-0000-7000-8000-0000000000b1")
	second := shared.MustParseID("018f2a1b-0000-7000-8000-0000000000b2")
	workspaces := listedWorkspaces{{ID: first}, {ID: second}}

	cases := []struct {
		name  string
		by    map[shared.ID]repository.Coverage
		want  []string
		count string
	}{
		{name: "no target anywhere", by: nil, want: []string{NotConfiguredCode}},
		{name: "one target in one workspace", by: map[shared.ID]repository.Coverage{second: {Configured: 1}},
			want: []string{SingleTargetCode}},
		{name: "one in each", by: map[shared.ID]repository.Coverage{first: {Configured: 1}, second: {Configured: 1}},
			want: nil},
		{name: "two of three unencrypted",
			by: map[shared.ID]repository.Coverage{
				first: {Configured: 2, Unencrypted: 1}, second: {Configured: 1, Unencrypted: 1},
			},
			want: []string{UnencryptedCode}, count: "2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := &TargetWarnings{
				Tenancy: env.TenancySingle, Workspaces: workspaces,
				Targets: &coverageByWorkspace{by: tc.by}, UnitOfWork: scopedWork{},
				Clock: stillClock{now: time.Unix(1_800_000_000, 0)},
			}
			source.Refresh(t.Context())
			settledTargets(t, source)

			got := codesOf(source.Warnings(t.Context()))
			if len(got) != len(tc.want) {
				t.Fatalf("warnings %v, want %v", got, tc.want)
			}
			for _, code := range tc.want {
				if _, ok := got[code]; !ok {
					t.Errorf("missing %s in %v", code, got)
				}
			}
			if tc.count != "" && got[UnencryptedCode].Params["count"] != tc.count {
				t.Errorf("count %q, want %q", got[UnencryptedCode].Params["count"], tc.count)
			}
		})
	}
}

// A count that was not taken states nothing about the targets, and says so (P-11); a failed count
// keeps the last one's warnings beside the warning that it is old.
func TestAnUncheckedCountIsSaidAndALastCountKept(t *testing.T) {
	clock := &movingTargetsClock{now: time.Unix(1_800_000_000, 0)}
	store := &coverageByWorkspace{err: errors.New("database gone")}
	source := &TargetWarnings{
		Tenancy: env.TenancySingle, Workspaces: listedWorkspaces{{ID: shared.MustParseID("018f2a1b-0000-7000-8000-0000000000b3")}},
		Targets: store, UnitOfWork: scopedWork{}, Clock: clock,
	}

	if got := codesOf(source.Warnings(t.Context())); got[CoverageUncheckedCode].Severity != "info" || len(got) != 1 {
		t.Errorf("before the first count: %v", got)
	}
	settledTargets(t, source)
	if got := codesOf(source.Warnings(t.Context())); got[CoverageUncheckedCode].Severity != "warn" || len(got) != 1 {
		t.Errorf("after a failed first count: %v", got)
	}
	settledTargets(t, source)

	store.mu.Lock()
	store.err = nil
	store.mu.Unlock()
	clock.advance(TargetWarningsEvery)
	source.Warnings(t.Context())
	settledTargets(t, source)
	if got := codesOf(source.Warnings(t.Context())); len(got) != 1 || got[NotConfiguredCode].Code == "" {
		t.Errorf("after a good count: %v", got)
	}

	store.mu.Lock()
	store.err = errors.New("database gone again")
	store.mu.Unlock()
	clock.advance(TargetWarningsEvery)
	source.Warnings(t.Context())
	settledTargets(t, source)
	got := codesOf(source.Warnings(t.Context()))
	if got[NotConfiguredCode].Code == "" || got[CoverageUncheckedCode].Severity != "warn" {
		t.Errorf("after a failed later count: %v", got)
	}
}

// In provider operation the installation's backup is its operator's (backup-restore.md §1), and one
// workspace's target says nothing about another's: no warning, and nothing counted.
func TestAProviderInstallationGetsNoBackupWarning(t *testing.T) {
	source := &TargetWarnings{
		Tenancy: env.TenancyMulti, Workspaces: listedWorkspaces{}, Targets: &coverageByWorkspace{},
		UnitOfWork: scopedWork{}, Clock: stillClock{},
	}
	source.Refresh(t.Context())
	if got := source.Warnings(t.Context()); len(got) != 0 {
		t.Errorf("a provider installation was warned: %v", got)
	}
	if source.running || source.counted {
		t.Error("a provider installation was counted")
	}
}

type movingTargetsClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *movingTargetsClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *movingTargetsClock) advance(by time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(by)
}
