// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package sealing

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	sealingrepo "github.com/Jersyfi/hubtask/core/application/repository/sealing"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/health"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// movingClock is a clock a test moves forward.
type movingClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *movingClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *movingClock) advance(by time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(by)
}

// lockedWork is the fake unit of work, safe to call from the background census.
type lockedWork struct {
	mu sync.Mutex
	unitOfWork
}

func (u *lockedWork) Within(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	u.mu.Lock()
	u.scopes = append(u.scopes, scope)
	u.mu.Unlock()
	return fn(ctx)
}

func (u *lockedWork) WithinReadOnly(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	return u.Within(ctx, scope, fn)
}

// levelCensus answers each level its own counts, and counts how often a workspace was asked.
type levelCensus struct {
	mu           sync.Mutex
	asked        int
	err          error
	workspace    sealingrepo.Counts
	installation sealingrepo.Counts
}

func (c *levelCensus) Count(context.Context) (sealingrepo.Counts, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.asked++
	return c.workspace, c.err
}

func (c *levelCensus) CountInstallation(context.Context) (sealingrepo.Counts, error) {
	return c.installation, nil
}

func (c *levelCensus) times() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.asked
}

// settled waits for the background census the last report started.
func settled(t *testing.T, k *KeyHealth) {
	t.Helper()
	for range 200 {
		k.mu.Lock()
		running := k.running
		k.mu.Unlock()
		if !running {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the background census never finished")
}

func codes(warnings []health.Warning) []string {
	listed := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		listed = append(listed, warning.Code+"/"+warning.Severity)
	}
	return listed
}

// UC-INS-16 check 3: a key the ring lacks while values still name it is named in the report with
// the kind each value is and how many - with the moment it was counted, never as if it were now.
func TestTheReportNamesWhatAMissingKeyWouldLeaveUnreadable(t *testing.T) {
	clock := &movingClock{now: now}
	census := &levelCensus{
		workspace:    counts("gone", "webhook_subscription", 2, "k1", "account_mfa", 4),
		installation: counts("gone", "identity_provider", 1),
	}
	keys := &KeyHealth{
		Tenants: tenants{listed: []adminrepo.TenantRecord{{ID: tenantOne}}}, Census: census,
		Encryptor: ring{keys: []string{"k2", "k1"}}, UnitOfWork: &lockedWork{}, Clock: clock,
	}

	// Before any count: unchecked, said quietly, and a count started.
	first := keys.Warnings(t.Context())
	if got := codes(first); len(got) != 1 || got[0] != KeyCensusUncheckedCode+"/info" {
		t.Errorf("before the first count: %v", got)
	}
	settled(t, keys)

	warnings := keys.Warnings(t.Context())
	if len(warnings) != 2 {
		t.Fatalf("warnings %+v", warnings)
	}
	stamp := now.UTC().Format(time.RFC3339)
	for i, want := range []map[string]string{
		{"key": "gone", "store": "identity_provider", "count": "1", "counted_at": stamp},
		{"key": "gone", "store": "webhook_subscription", "count": "2", "counted_at": stamp},
	} {
		got := warnings[i]
		if got.Code != KeyMissingCode || got.Severity != "warn" || len(got.Params) != len(want) {
			t.Errorf("warning %d: %+v", i, got)
		}
		for name, value := range want {
			if got.Params[name] != value {
				t.Errorf("warning %d: %s = %q, want %q", i, name, got.Params[name], value)
			}
		}
	}

	// Served from the count, not counted again, until it is an hour old.
	asked := census.times()
	clock.advance(30 * time.Minute)
	keys.Warnings(t.Context())
	settled(t, keys)
	if census.times() != asked {
		t.Error("a report within the hour counted again")
	}
	clock.advance(31 * time.Minute)
	keys.Warnings(t.Context())
	settled(t, keys)
	if census.times() != asked+1 {
		t.Error("a report after the hour did not count again")
	}

	// A count that fails keeps the last one, says how old it is, and says the count is unchecked.
	census.mu.Lock()
	census.err = errors.New("unreachable")
	census.mu.Unlock()
	clock.advance(2 * time.Hour)
	keys.Warnings(t.Context())
	settled(t, keys)
	after := keys.Warnings(t.Context())
	settled(t, keys)
	last := after[len(after)-1]
	if last.Code != KeyCensusUncheckedCode || last.Severity != "warn" || last.Params["counted_at"] == "" {
		t.Errorf("a failed count reads %+v", after)
	}
	if after[0].Params["counted_at"] == clock.Now().UTC().Format(time.RFC3339) {
		t.Error("an old count is shown as taken now")
	}
}

// The operator reading the key screen hands the report the count it just took.
func TestTheStatusReadRefreshesTheReport(t *testing.T) {
	keys := &KeyHealth{Encryptor: ring{keys: []string{"k2"}}, Clock: &movingClock{now: now}}
	work := &unitOfWork{}
	status := ReadEncryptionStatus{
		Tenants: tenants{listed: []adminrepo.TenantRecord{{ID: tenantOne}}},
		Census: census{work: work, counts: map[shared.ID]sealingrepo.Counts{
			tenantOne: counts("gone", "backup_target", 1),
		}},
		Encryptor: ring{keys: []string{"k2"}}, UnitOfWork: work, Observer: keys,
	}
	if _, err := status.Execute(t.Context(), operator()); err != nil {
		t.Fatal(err)
	}
	keys.mu.Lock()
	keys.running = true // no background count in this test: only what the read observed
	keys.mu.Unlock()
	got := keys.Warnings(t.Context())
	if len(got) != 1 || got[0].Params["store"] != "backup_target" {
		t.Errorf("after the status read the report says %+v", got)
	}
}
