// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
)

// UC-INS-15 checks 3, 4, 10 and 11 at the application layer: the owner's set-up and its refusals,
// the job that proves a domain by DNS through a fake resolver, the operator's block, and the trail
// and journal each leaves.

var otherWorkspace = shared.ID("018f2a1b-0000-7000-8000-0000000000e2")

// ownDomainStore is the table across workspaces; the workspace of each call is the unit of work's
// last scope, as row level security makes it in the store.
type ownDomainStore struct {
	uow     *unitOfWork
	rows    map[shared.ID]*domain.OwnDomain
	blocks  map[string]*blockRow
	looks   int
	taken   map[string]bool
	insertN int
}

type blockRow struct {
	block     domain.DomainBlock
	tenant    shared.ID
	dismissed bool
}

func newOwnDomainStore(uow *unitOfWork) *ownDomainStore {
	return &ownDomainStore{uow: uow, rows: map[shared.ID]*domain.OwnDomain{}, blocks: map[string]*blockRow{}, taken: map[string]bool{}}
}

func (s *ownDomainStore) tenant() shared.ID { return s.uow.scopes[len(s.uow.scopes)-1].TenantID }

func (s *ownDomainStore) Find(context.Context) (domain.OwnDomain, error) {
	if row, held := s.rows[s.tenant()]; held {
		return *row, nil
	}
	return domain.OwnDomain{}, shared.ErrNotFound
}

func (s *ownDomainStore) Insert(_ context.Context, host domain.TenantHost) error {
	if s.taken[host.Host] {
		return domain.OwnDomainTaken(host.Host)
	}
	for _, row := range s.rows {
		if row.Host == host.Host {
			return domain.OwnDomainTaken(host.Host)
		}
	}
	s.insertN++
	s.rows[s.tenant()] = &domain.OwnDomain{
		Host: host.Host, State: host.State, Verification: host.Verification, Since: host.CreatedAt,
	}
	return nil
}

func (s *ownDomainStore) Remove(context.Context) (domain.OwnDomain, error) {
	row, held := s.rows[s.tenant()]
	if !held {
		return domain.OwnDomain{}, shared.ErrNotFound
	}
	delete(s.rows, s.tenant())
	return *row, nil
}

func (s *ownDomainStore) Blocked(_ context.Context, host string) (bool, error) {
	_, held := s.blocks[host]
	return held, nil
}

func (s *ownDomainStore) BlockInView(context.Context) (domain.DomainBlock, error) {
	for _, b := range s.blocks {
		if b.tenant == s.tenant() && !b.dismissed {
			return b.block, nil
		}
	}
	return domain.DomainBlock{}, shared.ErrNotFound
}

func (s *ownDomainStore) DismissBlocks(context.Context, time.Time) (int, error) {
	n := 0
	for _, b := range s.blocks {
		if b.tenant == s.tenant() && !b.dismissed {
			b.dismissed = true
			n++
		}
	}
	return n, nil
}

func (s *ownDomainStore) Block(_ context.Context, block domain.DomainBlock) error {
	if _, held := s.blocks[block.Host]; held {
		return shared.ErrConflict.WithDetail("admin.own_domain_already_blocked")
	}
	s.blocks[block.Host] = &blockRow{block: block, tenant: s.tenant()}
	return nil
}

func (s *ownDomainStore) RecordLook(
	_ context.Context, host string, from, next domain.HostState, look domain.DomainLookup, at time.Time,
) (bool, error) {
	row, held := s.rows[s.tenant()]
	if !held || row.Host != host || row.State != from {
		return false, nil
	}
	s.looks++
	if row.State != next {
		row.Since = at
	}
	row.State, row.CheckedAt, row.TargetFound, row.ProofFound = next, at, look.TargetFound, look.ProofFound
	return true, nil
}

// Due answers every row the check owes a look; the store's timing is the SQL's, proven there.
func (s *ownDomainStore) Due(context.Context, time.Time, time.Time, int) ([]identityrepo.DueOwnDomain, error) {
	var due []identityrepo.DueOwnDomain
	for tenant, row := range s.rows {
		due = append(due, identityrepo.DueOwnDomain{TenantID: tenant, Host: row.Host})
	}
	return due, nil
}

func (s *ownDomainStore) All(context.Context) ([]identityrepo.ListedOwnDomain, error) {
	var all []identityrepo.ListedOwnDomain
	for tenant, row := range s.rows {
		all = append(all, identityrepo.ListedOwnDomain{Host: row.Host, TenantID: tenant, TenantName: "Meier family", State: row.State, Since: row.Since})
	}
	for host, b := range s.blocks {
		all = append(all, identityrepo.ListedOwnDomain{Host: host, TenantID: b.tenant, State: identityrepo.HostStateBlocked, BlockedReason: b.block.Reason})
	}
	return all, nil
}

func (s *ownDomainStore) Certificate(context.Context, string, time.Time) (bool, error) {
	return false, nil
}

// dnsFake answers from what a test put in its zone; a name in `failing` answers no answer at all.
type dnsFake struct {
	txt     map[string][]string
	cname   map[string]string
	failing map[string]bool
	asked   []string
}

func (d *dnsFake) TXT(_ context.Context, name string) ([]string, error) {
	d.asked = append(d.asked, name)
	if d.failing[name] {
		return nil, shared.ErrUnavailable.WithDetail("dependency.dns_unavailable")
	}
	return d.txt[name], nil
}

func (d *dnsFake) CanonicalName(_ context.Context, name string) (string, error) {
	d.asked = append(d.asked, name)
	if d.failing[name] {
		return "", shared.ErrUnavailable.WithDetail("dependency.dns_unavailable")
	}
	if target, held := d.cname[name]; held {
		return target, nil
	}
	return "", nil
}

type ownDomainFixture struct {
	shared     OwnDomains
	store      *ownDomainStore
	settings   *instanceStore
	audit      *auditSink
	journal    *journalStore
	authorizer *workspaceAuthorizerFake
	dns        *dnsFake
}

func newOwnDomainFixture(allowed bool) *ownDomainFixture {
	instance, settings, _ := newInstanceWriter(newRegister(operatorID))
	settings.level.OwnersOwnDomains = allowed
	uow := &unitOfWork{}
	f := &ownDomainFixture{
		store: newOwnDomainStore(uow), settings: settings, audit: &auditSink{}, journal: &journalStore{},
		authorizer: &workspaceAuthorizerFake{},
		dns:        &dnsFake{txt: map[string][]string{}, cname: map[string]string{}, failing: map[string]bool{}},
	}
	f.shared = OwnDomains{
		Instance: instance, Domains: f.store, Doors: f.store, Owners: ownersFake{owners: []shared.ID{addressOwner}},
		Settings: settings, Journal: f.journal, Audit: f.audit,
		Tenants:    &tenantsStore{record: adminrepo.TenantRecord{ID: lifecycleTenant, Slug: "meier", DisplayName: "Meier family"}},
		UnitOfWork: uow, Clock: clock.Fixed(now), IDs: &ids{}, Entropy: clock.FixedEntropy{},
		Registry: registryFake{}, Tenancy: env.TenancyMulti, InstallationHost: "tasks.example.net",
	}
	return f
}

type registryFake struct{}

func (registryFake) RegisteredDomain(host string) (string, error) {
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return "", errors.New("a public suffix")
	}
	return strings.Join(labels[len(labels)-2:], "."), nil
}

func (f *ownDomainFixture) setUp(t *testing.T, actor appshared.ActorContext, raw string) error {
	t.Helper()
	_, err := SetUpOwnDomain{OwnDomains: f.shared, Authorizer: f.authorizer}.Execute(t.Context(), actor, raw)
	return err
}

func (f *ownDomainFixture) check(t *testing.T) int {
	t.Helper()
	moved, err := CheckOwnDomains{OwnDomains: f.shared, Resolver: f.dns}.Run(t.Context())
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	return moved
}

func TestTheOwnerSetsUpAnOwnDomainAndIsAnsweredItsRecords(t *testing.T) {
	f := newOwnDomainFixture(true)
	out, err := SetUpOwnDomain{OwnDomains: f.shared, Authorizer: f.authorizer}.
		Execute(t.Context(), workspacePerson(addressOwner), "Tasks.Acme.com")
	if err != nil {
		t.Fatalf("set up: %v", err)
	}
	if out.String("domain") != "tasks.acme.com" || out.String("state") != "PENDING" {
		t.Fatalf("answered %v", out)
	}
	// Check 2: two records - the CNAME to the installation's target, the TXT carrying the mark.
	listed := recordsOf(t, out)
	if listed[0]["type"] != "CNAME" || listed[0]["name"] != "tasks.acme.com" || listed[0]["value"] != "domains.tasks.example.net" ||
		listed[1]["type"] != "TXT" || listed[1]["name"] != "_hubtask.tasks.acme.com" ||
		!strings.HasPrefix(listed[1]["value"].(string), "hubtask-domain=") {
		t.Errorf("records %v", listed)
	}
	// Check 11: adding is in the workspace's trail, the host and nothing else.
	if len(f.audit.entries) != 1 || f.audit.entries[0].Action != OwnDomainAddedAction ||
		f.audit.entries[0].TenantID != lifecycleTenant || f.audit.entries[0].ActorID != addressOwner {
		t.Errorf("trail %+v", f.audit.entries)
	}
}

func recordsOf(t *testing.T, out usecase.Output) []usecase.Output {
	t.Helper()
	listed, _ := out["records"].([]usecase.Output)
	if len(listed) != 2 {
		t.Fatalf("records %#v", out["records"])
	}
	return listed
}

// Check 1 and check 4: the installation's switch, the owner's role, and every refusal at the field.
func TestAnOwnDomainIsRefusedWhereItMayNotBe(t *testing.T) {
	t.Run("the installation does not allow it", func(t *testing.T) {
		f := newOwnDomainFixture(false)
		if err := f.setUp(t, workspacePerson(addressOwner), "tasks.acme.com"); detailOf(err) != "access.own_domains_not_allowed" {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("single mode", func(t *testing.T) {
		f := newOwnDomainFixture(true)
		f.shared.Tenancy = env.TenancySingle
		if err := f.setUp(t, workspacePerson(addressOwner), "tasks.acme.com"); detailOf(err) != "access.own_domains_not_allowed" {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("not an owner", func(t *testing.T) {
		f := newOwnDomainFixture(true)
		if err := f.setUp(t, workspacePerson(mailedMember), "tasks.acme.com"); detailOf(err) != "access.owner_required" {
			t.Errorf("err = %v", err)
		}
	})
	cases := map[string]string{
		"acme.tasks.example.net":  "admin.own_domain_installation",
		"acme.com":                "admin.own_domain_apex",
		"https://tasks acme":      "admin.own_domain_invalid",
		"taken.northwind.example": "admin.own_domain_taken",
		"acme-login.example.org":  "admin.own_domain_blocked",
	}
	for raw, code := range cases {
		t.Run(code, func(t *testing.T) {
			f := newOwnDomainFixture(true)
			f.store.taken["taken.northwind.example"] = true
			f.store.blocks["acme-login.example.org"] = &blockRow{tenant: otherWorkspace, block: domain.DomainBlock{Host: "acme-login.example.org", Reason: "x"}}
			err := f.setUp(t, workspacePerson(addressOwner), raw)
			problem := shared.AsError(err)
			if problem == nil || problem.DetailCode != code || len(problem.Fields) != 1 || problem.Fields[0].Path != "/domain" {
				t.Fatalf("set up %q: %v, want %s at /domain", raw, err, code)
			}
			if len(f.audit.entries) != 0 || f.store.insertN != 0 {
				t.Errorf("a refusal wrote: trail %d, rows %d", len(f.audit.entries), f.store.insertN)
			}
		})
	}
	t.Run("one per workspace", func(t *testing.T) {
		f := newOwnDomainFixture(true)
		if err := f.setUp(t, workspacePerson(addressOwner), "tasks.acme.com"); err != nil {
			t.Fatal(err)
		}
		if err := f.setUp(t, workspacePerson(addressOwner), "board.acme.com"); detailOf(err) != "admin.own_domain_exists" {
			t.Errorf("a second domain: %v", err)
		}
	})
}

// Check 3 and the acceptance's "pending to verified, verified to broken": the job alone moves the
// domain, by what the fake zone holds, and each move is in the workspace's trail.
func TestTheCheckVerifiesAndBreaksADomainByItself(t *testing.T) {
	f := newOwnDomainFixture(true)
	if err := f.setUp(t, workspacePerson(addressOwner), "tasks.acme.com"); err != nil {
		t.Fatal(err)
	}
	mark := f.store.rows[lifecycleTenant].Verification

	// Nothing in the zone yet: it waits, looked at.
	if moved := f.check(t); moved != 0 || f.store.rows[lifecycleTenant].State != domain.HostPending || f.store.looks != 1 {
		t.Fatalf("moved %d, state %s, looks %d", moved, f.store.rows[lifecycleTenant].State, f.store.looks)
	}
	// Only the proof: still waiting, the proof found.
	f.dns.txt["_hubtask.tasks.acme.com"] = []string{`"` + mark + `"`}
	f.check(t)
	if row := f.store.rows[lifecycleTenant]; row.State != domain.HostPending || !row.ProofFound || row.TargetFound {
		t.Fatalf("row %+v", row)
	}
	// Both records: verified.
	f.dns.cname["tasks.acme.com"] = "domains.tasks.example.net."
	if moved := f.check(t); moved != 1 || f.store.rows[lifecycleTenant].State != domain.HostVerified {
		t.Fatalf("moved %d, state %s", moved, f.store.rows[lifecycleTenant].State)
	}
	// A resolver's bad minute moves nothing.
	f.dns.failing["_hubtask.tasks.acme.com"] = true
	if moved := f.check(t); moved != 0 || f.store.rows[lifecycleTenant].State != domain.HostVerified {
		t.Fatalf("a failed lookup moved it to %s", f.store.rows[lifecycleTenant].State)
	}
	delete(f.dns.failing, "_hubtask.tasks.acme.com")
	// The proof gone: broken.
	delete(f.dns.txt, "_hubtask.tasks.acme.com")
	if moved := f.check(t); moved != 1 || f.store.rows[lifecycleTenant].State != domain.HostBroken {
		t.Fatalf("moved %d, state %s", moved, f.store.rows[lifecycleTenant].State)
	}
	// Back again, never asked for a certificate: verified.
	f.dns.txt["_hubtask.tasks.acme.com"] = []string{mark}
	if f.check(t); f.store.rows[lifecycleTenant].State != domain.HostVerified {
		t.Fatalf("state %s", f.store.rows[lifecycleTenant].State)
	}

	// Check 11: added, verified, broken, verified - each in the trail, by the system.
	var actions []audit.Action
	for _, entry := range f.audit.entries {
		actions = append(actions, entry.Action)
		if entry.Action != OwnDomainAddedAction && entry.ActorKind != shared.ActorSystem {
			t.Errorf("%s by %s", entry.Action, entry.ActorKind)
		}
	}
	want := []audit.Action{OwnDomainAddedAction, OwnDomainVerifiedAction, OwnDomainBrokenAction, OwnDomainVerifiedAction}
	if strings.Join(actionNames(actions), ",") != strings.Join(actionNames(want), ",") {
		t.Errorf("trail %v, want %v", actions, want)
	}
}

func actionNames(actions []audit.Action) []string {
	names := make([]string, 0, len(actions))
	for _, a := range actions {
		names = append(names, string(a))
	}
	return names
}

// A domain whose CNAME follows a chain is found where the target's own chain ends.
func TestTheCheckFollowsTheTargetsChain(t *testing.T) {
	f := newOwnDomainFixture(true)
	if err := f.setUp(t, workspacePerson(addressOwner), "tasks.acme.com"); err != nil {
		t.Fatal(err)
	}
	f.dns.txt["_hubtask.tasks.acme.com"] = []string{f.store.rows[lifecycleTenant].Verification}
	f.dns.cname["domains.tasks.example.net"] = "lb.provider.example."
	f.dns.cname["tasks.acme.com"] = "lb.provider.example."
	if f.check(t); f.store.rows[lifecycleTenant].State != domain.HostVerified {
		t.Fatalf("state %s", f.store.rows[lifecycleTenant].State)
	}
}

// Check 10 and 11: the operator blocks a domain with a reason; it stops answering at once (its row
// is gone), the owner sees the reason, no workspace sets it up again, and the block is in the
// workspace's trail without the reason and in the instance journal with it. The check passes over
// a blocked domain: there is no row left to look at.
func TestTheOperatorBlocksAnOwnDomain(t *testing.T) {
	f := newOwnDomainFixture(true)
	if err := f.setUp(t, workspacePerson(addressOwner), "tasks.acme.com"); err != nil {
		t.Fatal(err)
	}
	block := BlockOwnDomain{OwnDomains: f.shared}
	if _, err := block.Execute(t.Context(), operator(), "tasks.acme.com", "  "); detailOf(err) != "admin.own_domain_reason_required" {
		t.Fatalf("without a reason: %v", err)
	}
	if _, err := block.Execute(t.Context(), workspacePerson(addressOwner), "tasks.acme.com", "x"); err == nil {
		t.Fatal("an owner blocked a domain")
	}
	reason := "Reported as a look-alike of another company's sign-in page."
	blocked, err := block.Execute(t.Context(), operator(), "tasks.acme.com", reason)
	if err != nil {
		t.Fatalf("block: %v", err)
	}
	if blocked.State != identityrepo.HostStateBlocked || blocked.BlockedReason != reason || blocked.TenantID != lifecycleTenant {
		t.Errorf("answered %+v", blocked)
	}
	if _, held := f.store.rows[lifecycleTenant]; held {
		t.Error("the blocked domain still has its row, so it still answers")
	}
	last := f.audit.entries[len(f.audit.entries)-1]
	if last.Action != OwnDomainBlockedAction || last.TenantID != lifecycleTenant || last.ActorID != operatorID {
		t.Errorf("trail %+v", last)
	}
	if strings.Contains(fmt.Sprint(last.Changes), "look-alike") {
		t.Error("the reason reached the trail")
	}
	if len(f.journal.entries) != 1 || f.journal.entries[0].Action != journalOwnDomainBlocked ||
		f.journal.entries[0].Details["reason"] != reason || f.journal.entries[0].Details["domain"] != "tasks.acme.com" {
		t.Errorf("journal %+v", f.journal.entries)
	}
	if _, err := block.Execute(t.Context(), operator(), "tasks.acme.com", reason); detailOf(err) != "admin.own_domain_already_blocked" {
		t.Errorf("blocked twice: %v", err)
	}
	if f.check(t); f.store.looks != 0 {
		t.Errorf("the check looked at a blocked domain %d times", f.store.looks)
	}

	// The owner sees the block until removing it, and cannot set up another meanwhile.
	f.store.uow.scopes = append(f.store.uow.scopes, workspacePerson(addressOwner).PersistenceScope())
	if seen, err := f.store.BlockInView(t.Context()); err != nil || seen.Reason != reason {
		t.Errorf("the owner sees %+v, %v", seen, err)
	}
	if err := f.setUp(t, workspacePerson(addressOwner), "board.acme.com"); detailOf(err) != "admin.own_domain_exists" {
		t.Errorf("set up beside a block: %v", err)
	}
	if err := (RemoveOwnDomain{OwnDomains: f.shared, Authorizer: f.authorizer}).Execute(t.Context(), workspacePerson(addressOwner)); err != nil {
		t.Fatalf("remove: %v", err)
	}
	// No workspace sets it up again.
	if err := f.setUp(t, workspacePerson(addressOwner), "tasks.acme.com"); detailOf(err) != "admin.own_domain_blocked" {
		t.Errorf("set up again: %v", err)
	}
}

func TestTheOwnerRemovesTheOwnDomain(t *testing.T) {
	f := newOwnDomainFixture(true)
	remove := RemoveOwnDomain{OwnDomains: f.shared, Authorizer: f.authorizer}
	if err := remove.Execute(t.Context(), workspacePerson(addressOwner)); detailOf(err) != "admin.own_domain_none" {
		t.Fatalf("nothing to remove: %v", err)
	}
	if err := f.setUp(t, workspacePerson(addressOwner), "tasks.acme.com"); err != nil {
		t.Fatal(err)
	}
	if err := remove.Execute(t.Context(), workspacePerson(mailedMember)); detailOf(err) != "access.owner_required" {
		t.Errorf("a member removed it: %v", err)
	}
	if err := remove.Execute(t.Context(), workspacePerson(addressOwner)); err != nil {
		t.Fatal(err)
	}
	if len(f.store.rows) != 0 || f.audit.entries[len(f.audit.entries)-1].Action != OwnDomainRemovedAction {
		t.Errorf("rows %v, trail %+v", f.store.rows, f.audit.entries)
	}
}

// Check 10: the operator's list names every own domain with its workspace and state.
func TestTheOperatorSeesEveryOwnDomain(t *testing.T) {
	f := newOwnDomainFixture(true)
	if err := f.setUp(t, workspacePerson(addressOwner), "tasks.acme.com"); err != nil {
		t.Fatal(err)
	}
	listed, err := ListOwnDomains{OwnDomains: f.shared}.Execute(t.Context(), operator())
	if err != nil || len(listed) != 1 || listed[0].Host != "tasks.acme.com" || listed[0].TenantID != lifecycleTenant ||
		listed[0].State != domain.HostPending {
		t.Fatalf("listed %+v, %v", listed, err)
	}
	if _, err := (ListOwnDomains{OwnDomains: f.shared}).Execute(t.Context(), workspacePerson(addressOwner)); err == nil {
		t.Error("an owner read the installation's list")
	}
}
