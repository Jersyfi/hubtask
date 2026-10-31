// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	ldapport "github.com/Jersyfi/hubtask/core/port/ldap"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const ldapServiceID = shared.ID("01936f2a-7c1e-7000-8000-0000000000c1")

// ldapStore is migration 0285's row, in memory, with whom it holds read off the directory links.
type ldapStore struct {
	links     *directoryLinks
	groups    *groupStore
	directory *domain.LdapDirectory
	sealed    cryptoport.Sealed
	runs      int
}

func (s *ldapStore) Find(context.Context) (domain.LdapDirectory, cryptoport.Sealed, error) {
	if s.directory == nil {
		return domain.LdapDirectory{}, cryptoport.Sealed{}, shared.ErrNotFound.WithDetail("directory.ldap_not_configured")
	}
	return *s.directory, s.sealed, nil
}

func (s *ldapStore) Save(_ context.Context, d domain.LdapDirectory, sealed cryptoport.Sealed, _ shared.ID) error {
	s.directory, s.sealed = &d, sealed
	return nil
}

func (s *ldapStore) RecordRun(_ context.Context, d domain.LdapDirectory) error {
	if s.directory == nil {
		return shared.ErrNotFound.WithDetail("directory.ldap_not_configured")
	}
	s.runs++
	s.directory = &d
	return nil
}

func (s *ldapStore) Delete(context.Context) error {
	if s.directory == nil {
		return shared.ErrNotFound.WithDetail("directory.ldap_not_configured")
	}
	s.directory = nil
	return nil
}

func (s *ldapStore) Held(context.Context) ([]domain.LdapHeld, error) {
	var held []domain.LdapHeld
	for id, external := range s.links.people {
		account := s.links.accounts.byID[id]
		if strings.HasPrefix(external, domain.LdapExternalPrefix) {
			held = append(held, domain.LdapHeld{AccountID: id, ExternalID: external, Name: account.DisplayName, Status: account.Status})
		}
	}
	return held, nil
}

func (s *ldapStore) HeldGroups(context.Context) ([]domain.LdapHeldGroup, error) {
	var held []domain.LdapHeldGroup
	for id, external := range s.links.groups {
		if strings.HasPrefix(external, domain.LdapExternalPrefix) {
			held = append(held, domain.LdapHeldGroup{GroupID: id, ExternalID: external})
		}
	}
	return held, nil
}

var _ repository.LdapDirectories = (*ldapStore)(nil)

// ldapReader is the directory: what it answers, whether it takes the bind.
type ldapReader struct {
	answer  domain.LdapAnswer
	refuse  error
	probes  int
	reads   int
	secrets []string
}

func (r *ldapReader) Probe(_ context.Context, c ldapport.Connection) error {
	r.probes++
	r.secrets = append(r.secrets, c.Password.Reveal())
	return r.refuse
}

func (r *ldapReader) Read(_ context.Context, c ldapport.Connection) (domain.LdapAnswer, error) {
	r.reads++
	r.secrets = append(r.secrets, c.Password.Reveal())
	return r.answer, r.refuse
}

// sealer seals by prefixing the purpose, so a ciphertext opened under another purpose fails.
type sealer struct{}

func (sealer) Seal(_ context.Context, plain secret.Secret, purpose cryptoport.Purpose) (cryptoport.Sealed, error) {
	return cryptoport.Sealed{KeyID: "k", Ciphertext: []byte(string(purpose) + "|" + plain.Reveal())}, nil
}

func (sealer) Open(_ context.Context, sealed cryptoport.Sealed, purpose cryptoport.Purpose) (secret.Secret, error) {
	before, after, ok := strings.Cut(string(sealed.Ciphertext), "|")
	if !ok || before != string(purpose) {
		return secret.Secret{}, shared.ErrInternal.WithDetail("crypto.open_failed")
	}
	return secret.New(after), nil
}

type ldapJobs struct {
	mu       sync.Mutex
	requests []queue.Request
}

func (j *ldapJobs) Enqueue(_ context.Context, r queue.Request) (shared.ID, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.requests = append(j.requests, r)
	return "job", nil
}

// seqIDs answers a new identifier each time, so a run that provisions two people makes two.
type seqIDs struct{ n *int }

func (s seqIDs) NewID() shared.ID {
	*s.n++
	return shared.ID("01936f2a-7c1e-7000-8000-" + strings.Repeat("0", 10) + string(rune('a'+*s.n/10)) + string(rune('0'+*s.n%10)))
}

type ldapFixture struct {
	directoryFixture
	store  *ldapStore
	reader *ldapReader
	jobs   *ldapJobs
	stepUp *stepUpFake
	auth   *authorizer
	ldap   LdapDirectories
}

func newLdap(existing ...domain.Account) *ldapFixture {
	service := domain.Account{ID: ldapServiceID, TenantID: tenant, Kind: domain.AccountServiceAccount, DisplayName: "Directory", Status: domain.AccountActive}
	f := &ldapFixture{directoryFixture: newDirectory(append(existing, service)...), reader: &ldapReader{}, jobs: &ldapJobs{}, stepUp: &stepUpFake{satisfied: true}}
	n := 0
	f.service.IDs = seqIDs{n: &n}
	f.auth = f.service.Authorizer.(*authorizer)
	f.store = &ldapStore{links: f.links, groups: f.groups}
	f.ldap = LdapDirectories{
		Store: f.store, Reader: f.reader, Encryptor: sealer{}, Directory: f.service, StepUp: f.stepUp, Jobs: f.jobs,
	}
	return f
}

func ldapConfiguration() usecase.Input {
	return usecase.Input{
		"url": "ldaps://dc.example.com", "bind_dn": "cn=reader,dc=example,dc=com", "bind_password": "s3cret",
		"user_base": "ou=people,dc=example,dc=com", "run_as": ldapServiceID.String(), "step_up_token": "proof",
	}
}

func (f *ldapFixture) configure(t *testing.T) {
	t.Helper()
	if _, err := (ConfigureLdapDirectory{Directories: f.ldap}).invoke(t.Context(), admin(), ldapConfiguration()); err != nil {
		t.Fatalf("configuring: %v", err)
	}
}

func (f *ldapFixture) sync(t *testing.T, in usecase.Input) usecase.Output {
	t.Helper()
	out, err := (SyncLdapDirectory{Directories: f.ldap}).invoke(t.Context(), admin(), in)
	if err != nil {
		t.Fatalf("syncing: %v", err)
	}
	return out
}

func detailCode(err error) string {
	if coded := shared.AsError(err); coded != nil {
		return coded.DetailCode
	}
	return ""
}

func person(id shared.ID, email, name string, status domain.AccountStatus) domain.Account {
	return domain.Account{ID: id, TenantID: tenant, Kind: domain.AccountUser, Email: email, DisplayName: name, Status: status}
}

// UC-ID-31 check 1: the directory is bound to before it is stored, the password sealed to the
// workspace and never answered, and an omitted password keeps the sealed one.
func TestAnLdapDirectoryIsBoundToBeforeItIsStored(t *testing.T) {
	f := newLdap()
	f.reader.refuse = shared.ErrValidation.WithDetail("directory.ldap_refused")
	_, err := (ConfigureLdapDirectory{Directories: f.ldap}).invoke(t.Context(), admin(), ldapConfiguration())
	if detailCode(err) != "directory.ldap_refused" || f.store.directory != nil {
		t.Fatalf("a refused bind: %v, stored %v", err, f.store.directory != nil)
	}
	f.reader.refuse = nil
	f.configure(t)
	if string(f.store.sealed.Ciphertext) != string(LdapPasswordPurpose(tenant))+"|s3cret" {
		t.Fatalf("sealed %q, want the password bound to the workspace", f.store.sealed.Ciphertext)
	}
	if len(f.jobs.requests) != 1 || f.jobs.requests[0].Kind != queue.KindLdapSync || f.jobs.requests[0].DedupeKey != tenant.String() {
		t.Fatalf("the first run was not queued: %+v", f.jobs.requests)
	}
	in := ldapConfiguration()
	delete(in, "bind_password")
	in["user_filter"] = "(objectClass=inetOrgPerson)"
	out, err := (ConfigureLdapDirectory{Directories: f.ldap}).invoke(t.Context(), admin(), in)
	if err != nil {
		t.Fatal(err)
	}
	if f.reader.secrets[len(f.reader.secrets)-1] != "s3cret" || f.store.directory.UserFilter != "(objectClass=inetOrgPerson)" {
		t.Fatalf("an omitted password was not the sealed one: %v", f.reader.secrets)
	}
	for key := range out {
		if strings.Contains(key, "password") && key != "has_password" {
			t.Fatalf("the answer carries %s", key)
		}
	}
	if !strings.Contains(string(f.sink.entries[len(f.sink.entries)-1].Action), "ldap_configured") {
		t.Fatal("the change was not audited")
	}
}

// UC-ID-31 check 1: the change demands a step-up, and the run acts only as a service account
// of the workspace.
func TestAnLdapDirectoryNeedsAStepUpAndAServiceAccount(t *testing.T) {
	f := newLdap(person(adminID, "anna@example.com", "Anna", domain.AccountActive))
	f.stepUp.satisfied = false
	if _, err := (ConfigureLdapDirectory{Directories: f.ldap}).invoke(t.Context(), admin(), ldapConfiguration()); detailCode(err) != "auth.step_up_required" {
		t.Fatalf("without a step-up: %v", err)
	}
	f.stepUp.satisfied = true
	in := ldapConfiguration()
	in["run_as"] = adminID.String()
	if _, err := (ConfigureLdapDirectory{Directories: f.ldap}).invoke(t.Context(), admin(), in); detailCode(err) != "directory.ldap_run_as_invalid" {
		t.Fatalf("a person as the service account: %v", err)
	}
	if f.reader.probes != 0 || f.store.directory != nil {
		t.Fatal("the directory was bound to or stored")
	}
}

// UC-ID-31 checks 2 and 3: a run invites who is new, renames, switches off who left - never an
// owner - and on who returned, as the service account.
func TestAnLdapRunKeepsThePeopleAsTheDirectoryDoes(t *testing.T) {
	leaving := person("01936f2a-7c1e-7000-8000-0000000000d1", "max@example.com", "Max", domain.AccountActive)
	owner := person("01936f2a-7c1e-7000-8000-0000000000d2", "olga@example.com", "Olga", domain.AccountActive)
	back := person("01936f2a-7c1e-7000-8000-0000000000d3", "ben@example.com", "Ben", domain.AccountDisabled)
	renamed := person("01936f2a-7c1e-7000-8000-0000000000d4", "rita@example.com", "Rita", domain.AccountActive)
	f := newLdap(leaving, owner, back, renamed)
	f.links.people[leaving.ID], f.links.people[owner.ID] = "ldap:max", "ldap:olga"
	f.links.people[back.ID], f.links.people[renamed.ID] = "ldap:ben", "ldap:rita"
	f.owners.paths = []repository.OwnerPath{{AccountID: owner.ID}}
	f.configure(t)
	f.reader.answer = domain.LdapAnswer{People: []domain.LdapPerson{
		{DN: "uid=lena", ID: "lena", Email: "Lena@Example.com", Name: "Lena"},
		{DN: "uid=ben", ID: "ben", Email: "ben@example.com", Name: "Ben"},
		{DN: "uid=rita", ID: "rita", Email: "rita@example.com", Name: "Rita Weber"},
	}}
	out := f.sync(t, usecase.Input{})
	counts := out["counts"].(usecase.Output)
	if out["outcome"] != "OK" || counts["provisioned"] != 1 || counts["switched_off"] != 1 || counts["kept"] != 1 ||
		counts["switched_on"] != 1 || counts["renamed"] != 1 {
		t.Fatalf("the run: %+v", out)
	}
	if f.accounts.byID[leaving.ID].Status != domain.AccountDisabled || f.accounts.byID[owner.ID].Status != domain.AccountActive ||
		f.accounts.byID[back.ID].Status != domain.AccountActive {
		t.Fatalf("statuses: max %s olga %s ben %s", f.accounts.byID[leaving.ID].Status, f.accounts.byID[owner.ID].Status, f.accounts.byID[back.ID].Status)
	}
	lena, err := f.accounts.FindByEmail(t.Context(), "lena@example.com")
	if err != nil || lena.Status != domain.AccountInvited || f.links.people[lena.ID] != "ldap:lena" {
		t.Fatalf("Lena: %+v, %v", lena, err)
	}
	if f.store.directory.LastOutcome != domain.LdapOK || f.store.runs != 1 {
		t.Fatalf("the run was not recorded: %+v", f.store.directory)
	}
	if _, ok := f.accounts.byID[leaving.ID]; !ok {
		t.Fatal("a person who left was erased")
	}
}

// UC-ID-31 check 4: a directory that answers nobody, or would switch off too many, changes nothing;
// the count given back with a step-up lets the run through.
func TestAnLdapRunRefusesAnAnswerThatWouldEmptyTheWorkspace(t *testing.T) {
	var people []domain.Account
	for i := range 12 {
		id := shared.ID("01936f2a-7c1e-7000-8000-0000000001" + string(rune('a'+i/10)) + string(rune('0'+i%10)))
		people = append(people, person(id, "p"+string(rune('a'+i))+"@example.com", "P", domain.AccountActive))
	}
	f := newLdap(people...)
	for i, p := range people {
		f.links.people[p.ID] = "ldap:p" + string(rune('a'+i))
	}
	f.configure(t)

	out := f.sync(t, usecase.Input{})
	if out["outcome"] != "REFUSED" || out["error"] != "directory.ldap_answer_empty" {
		t.Fatalf("an empty answer: %+v", out)
	}
	f.reader.answer = domain.LdapAnswer{People: []domain.LdapPerson{{ID: "pa", Email: "pa@example.com", Name: "P"}}}
	out = f.sync(t, usecase.Input{})
	if out["outcome"] != "REFUSED" || out["error"] != "directory.ldap_switch_off_too_many" || out["switch_off_planned"] != 11 {
		t.Fatalf("too many: %+v", out)
	}
	for _, p := range people {
		if f.accounts.byID[p.ID].Status != domain.AccountActive {
			t.Fatal("a refused run switched somebody off")
		}
	}
	if f.store.directory.Failures != 2 {
		t.Fatalf("failures %d, want the two refusals counted", f.store.directory.Failures)
	}

	f.stepUp.satisfied = false
	if _, err := (SyncLdapDirectory{Directories: f.ldap}).invoke(t.Context(), admin(), usecase.Input{"confirm_switch_off": 11}); detailCode(err) != "auth.step_up_required" {
		t.Fatalf("a confirmation without a step-up: %v", err)
	}
	f.stepUp.satisfied = true
	dry := f.sync(t, usecase.Input{"confirm_switch_off": 11, "dry_run": true, "step_up_token": "proof"})
	if dry["dry_run"] != true || dry["counts"].(usecase.Output)["switched_off"] != 11 || f.accounts.byID[people[1].ID].Status != domain.AccountActive {
		t.Fatalf("a dry run changed something or planned nothing: %+v", dry)
	}
	out = f.sync(t, usecase.Input{"confirm_switch_off": 11, "step_up_token": "proof"})
	if out["outcome"] != "OK" || out["counts"].(usecase.Output)["switched_off"] != 11 {
		t.Fatalf("a confirmed run: %+v", out)
	}
}

// UC-ID-31 check 5: the scheduled run records a directory that is down and comes back after the
// backoff; a directory removed ends it.
func TestTheScheduledLdapRunHealsAndEnds(t *testing.T) {
	f := newLdap()
	f.configure(t)
	f.reader.refuse = shared.ErrUnavailable.WithDetail("directory.ldap_unreachable")
	job := LdapSyncJob{Directories: f.ldap}
	result, err := job.Run(t.Context(), queue.Job{TenantID: tenant, Kind: queue.KindLdapSync})
	if err != nil || !result.Repeat || result.RepeatAfter < 2*60*60*1e9 {
		t.Fatalf("a directory that is down: %+v, %v", result, err)
	}
	if f.store.directory.LastOutcome != domain.LdapFailed || f.store.directory.LastError != "directory.ldap_unreachable" {
		t.Fatalf("the failure was not recorded: %+v", f.store.directory)
	}
	// Not due yet: it waits without reading.
	reads := f.reader.reads
	result, err = job.Run(t.Context(), queue.Job{TenantID: tenant, Kind: queue.KindLdapSync})
	if err != nil || !result.Repeat || f.reader.reads != reads {
		t.Fatalf("a run not due: %+v, %v", result, err)
	}
	if _, err := (RemoveLdapDirectory{Directories: f.ldap}).invoke(t.Context(), admin(), usecase.Input{"step_up_token": "proof"}); err != nil {
		t.Fatal(err)
	}
	result, err = job.Run(t.Context(), queue.Job{TenantID: tenant, Kind: queue.KindLdapSync})
	if err != nil || result.Repeat || result.Outcome != "removed" {
		t.Fatalf("a directory removed: %+v, %v", result, err)
	}
	if _, err := (ShowLdapDirectory{Directories: f.ldap}).invoke(t.Context(), admin(), usecase.Input{}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("shown after removal: %v", err)
	}
}

// UC-ID-31 check 6: groups follow the directory's members.
func TestAnLdapRunKeepsTheGroupsMembers(t *testing.T) {
	member := person("01936f2a-7c1e-7000-8000-0000000000e1", "mia@example.com", "Mia", domain.AccountActive)
	f := newLdap(member)
	f.links.people[member.ID] = "ldap:mia"
	f.configure(t)
	f.store.directory.GroupBase = "ou=groups,dc=example,dc=com"
	f.reader.answer = domain.LdapAnswer{
		People: []domain.LdapPerson{{DN: "uid=mia,ou=people", ID: "mia", Email: "mia@example.com", Name: "Mia"}},
		Groups: []domain.LdapGroup{{ID: "eng", Name: "Engineering", MemberNames: []string{"UID=mia, ou=people"}}},
	}
	out := f.sync(t, usecase.Input{})
	if out["outcome"] != "OK" || out["counts"].(usecase.Output)["groups_created"] != 1 {
		t.Fatalf("the run: %+v", out)
	}
	members, _ := f.groups.Members(t.Context(), groupID)
	if len(members) != 1 || members[0] != member.ID || f.links.groups[groupID] != "ldap:eng" {
		t.Fatalf("the group: %v, %q", members, f.links.groups[groupID])
	}
	// A second run that finds nothing changed writes nothing.
	out = f.sync(t, usecase.Input{})
	if out["counts"].(usecase.Output)["groups_changed"] != 0 {
		t.Fatalf("an unchanged group was written: %+v", out)
	}
}
