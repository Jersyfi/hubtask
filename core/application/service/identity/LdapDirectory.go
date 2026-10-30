// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	ldapport "github.com/Jersyfi/hubtask/core/port/ldap"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	stepupport "github.com/Jersyfi/hubtask/core/port/stepup"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// An LDAP directory keeps the workspace's people and groups on a schedule (UC-ID-31, identity.md
// §27). Every change a run makes goes through Directory, acting as the configured service account,
// so SCIM's rules hold unchanged: the same permission, the people limit, an owner never switched
// off, a restricted person never touched, nobody erased, a group holding OWNER never changed.

// The use case names.
const (
	ShowLdapDirectoryName      = "ShowLdapDirectory"
	ConfigureLdapDirectoryName = "ConfigureLdapDirectory"
	RemoveLdapDirectoryName    = "RemoveLdapDirectory"
	SyncLdapDirectoryName      = "SyncLdapDirectory"
)

// The stable audit codes (audit.md §2). Neither the password nor a person's name reaches the
// trail; a run records its counts (rule 10).
const (
	LdapConfiguredAction audit.Action = "directory.ldap_configured"
	LdapRemovedAction    audit.Action = "directory.ldap_removed"
	LdapSyncedAction     audit.Action = "directory.ldap_synced"
	LdapReadAction       audit.Action = "directory.ldap_read"

	ldapTarget = "ldap_directory"
	// ldapProbeTimeout bounds the bind an administrator waits for; ldapRunTimeout a whole run.
	ldapProbeTimeout = 30 * time.Second
	ldapRunTimeout   = 15 * time.Minute
)

// LdapPasswordPurpose binds the sealed bind password to its workspace, so that a ciphertext lifted
// into another workspace's row does not open.
func LdapPasswordPurpose(tenantID shared.ID) cryptoport.Purpose {
	return cryptoport.Purpose("ldap.bind_password:" + tenantID.String())
}

// LdapSealer is the slice of the installation's encryptor the bind password needs.
type LdapSealer interface {
	Seal(ctx context.Context, plaintext secret.Secret, purpose cryptoport.Purpose) (cryptoport.Sealed, error)
	Open(ctx context.Context, sealed cryptoport.Sealed, purpose cryptoport.Purpose) (secret.Secret, error)
}

// LdapJobs is the queue's slice a directory's schedule seeds its run into.
type LdapJobs interface {
	Enqueue(ctx context.Context, request queue.Request) (shared.ID, error)
}

// LdapDirectories is what the LDAP use cases and the scheduled run share.
type LdapDirectories struct {
	Store     repository.LdapDirectories
	Reader    ldapport.Reader
	Encryptor LdapSealer
	// Directory is SCIM's service; its authoriser, trail, transactions and clock are this one's.
	Directory Directory
	// StepUp is the proof a change to the directory demands, as one to a way in does: whoever sets
	// the directory decides who is in the workspace. Nil demands nothing, on a build without it.
	StepUp stepupport.Verifier
	Jobs   LdapJobs
}

// ShowLdapDirectory answers the directory and its last run.
type ShowLdapDirectory struct{ Directories LdapDirectories }

// ConfigureLdapDirectory sets the directory, bound to once before it is stored.
type ConfigureLdapDirectory struct{ Directories LdapDirectories }

// RemoveLdapDirectory forgets the directory and its password.
type RemoveLdapDirectory struct{ Directories LdapDirectories }

// SyncLdapDirectory runs the synchronisation now, or plans it.
type SyncLdapDirectory struct{ Directories LdapDirectories }

// LdapSyncJob is the scheduled run (queue.KindLdapSync).
type LdapSyncJob struct{ Directories LdapDirectories }

// LdapSyncRequest is the queued run of a workspace's directory, due at the moment given.
func LdapSyncRequest(tenantID shared.ID, at time.Time) queue.Request {
	return queue.Request{
		Kind: queue.KindLdapSync, TenantID: tenantID, DedupeKey: tenantID.String(), RunAt: at.UTC(),
		Payload: map[string]any{},
	}
}

func (l LdapDirectories) authorize(
	ctx context.Context, actor appshared.ActorContext, action audit.Action, scope string,
) error {
	return l.Directory.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []domain.Scope{domain.TenantScope()},
		Action:     action,
		TokenScope: scope,
		TargetType: ldapTarget,
	})
}

func (l LdapDirectories) proveChange(ctx context.Context, actor appshared.ActorContext, token string) error {
	if l.StepUp == nil {
		return nil
	}
	return stepupport.Demand(ctx, l.StepUp, actor.TenantID, actor.AccountID, token)
}

// runAs is the actor a run acts as: the configured service account, with the scopes SCIM's token
// holds and nothing else.
func runAs(tenantID, account shared.ID) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorServiceAccount, TenantID: tenantID, AccountID: account,
		Scopes: []string{membersRead, membersWrite},
	}
}

func (l LdapDirectories) find(ctx context.Context, scope persistence.Scope) (domain.LdapDirectory, cryptoport.Sealed, error) {
	var (
		d      domain.LdapDirectory
		sealed cryptoport.Sealed
	)
	err := l.Directory.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		var err error
		d, sealed, err = l.Store.Find(ctx)
		return err
	})
	if err == nil {
		d.TenantID = scope.TenantID
	}
	return d, sealed, err
}

func (l LdapDirectories) seed(ctx context.Context, d domain.LdapDirectory) error {
	if l.Jobs == nil || d.NextRunAt == nil {
		return nil
	}
	_, err := l.Jobs.Enqueue(ctx, LdapSyncRequest(d.TenantID, *d.NextRunAt))
	return err
}

func (h ShowLdapDirectory) invoke(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
	l := h.Directories
	if err := l.authorize(ctx, actor, LdapReadAction, identityProviderManage); err != nil {
		return nil, err
	}
	d, _, err := l.find(ctx, actor.PersistenceScope())
	if err != nil {
		return nil, err
	}
	// A run more than a day overdue lost its job somewhere: it is queued again, so the schedule
	// heals the moment anybody looks (the dedupe key keeps it one).
	if d.Enabled && d.NextRunAt != nil && l.Directory.Clock.Now().Sub(*d.NextRunAt) > 24*time.Hour {
		_ = l.seed(ctx, d)
	}
	return LdapDirectoryOutput(d, true), nil
}

func (h ConfigureLdapDirectory) invoke(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	l := h.Directories
	if err := l.authorize(ctx, actor, LdapConfiguredAction, identityProviderManage); err != nil {
		return nil, err
	}
	if err := l.proveChange(ctx, actor, in.String("step_up_token")); err != nil {
		return nil, err
	}
	runAsID, err := in.ID("run_as")
	if err != nil {
		return nil, ldapRefusal("directory.ldap_run_as_required", "/run_as")
	}
	var enabled *bool
	if in.Present("enabled") {
		given := in.Bool("enabled")
		enabled = &given
	}
	now := l.Directory.Clock.Now()
	d, err := domain.NewLdapDirectory(actor.TenantID, domain.LdapDirectoryInput{
		URL: in.String("url"), BindDN: in.String("bind_dn"), UserBase: in.String("user_base"),
		UserFilter: in.String("user_filter"), GroupBase: in.String("group_base"), GroupFilter: in.String("group_filter"),
		IDAttribute: in.String("id_attribute"), MailAttribute: in.String("mail_attribute"),
		NameAttribute: in.String("name_attribute"), GroupNameAttribute: in.String("group_name_attribute"),
		MemberAttribute: in.String("member_attribute"), RunAs: runAsID, EveryMinutes: in.Int("every_minutes"),
		Enabled: enabled,
	}, now)
	if err != nil {
		return nil, err
	}
	password := secret.New(in.String("bind_password"))
	if len(password.Reveal()) > domain.MaxLdapPassword {
		return nil, ldapRefusal("directory.ldap_password_too_long", "/bind_password")
	}
	stored, sealed, err := l.find(ctx, actor.PersistenceScope())
	exists := err == nil
	if err != nil && !errors.Is(err, shared.ErrNotFound) {
		return nil, err
	}
	if exists {
		d = d.Reconfigured(stored)
	}
	if password.IsEmpty() {
		if !exists {
			return nil, ldapRefusal("directory.ldap_password_required", "/bind_password")
		}
		// Omitted keeps the sealed one: the only way to change a filter without retyping a
		// password nothing can read back.
		if password, err = l.Encryptor.Open(ctx, sealed, LdapPasswordPurpose(actor.TenantID)); err != nil {
			return nil, err
		}
	}
	if err := l.checkRunAs(ctx, actor, runAsID); err != nil {
		return nil, err
	}
	// Bound to before anything is stored: a directory that cannot be read is said now, in the
	// form, and not at the first run.
	probe, cancel := context.WithTimeout(ctx, ldapProbeTimeout)
	defer cancel()
	if err := l.Reader.Probe(probe, ldapport.Connection{Directory: d, Password: password}); err != nil {
		return nil, err
	}
	sealed, err = l.Encryptor.Seal(ctx, password, LdapPasswordPurpose(actor.TenantID))
	if err != nil {
		return nil, err
	}
	err = l.Directory.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := l.Store.Save(ctx, d, sealed, actor.AccountID); err != nil {
			return err
		}
		return l.record(ctx, actor, LdapConfiguredAction,
			audit.Change{Field: "url", Classification: audit.Open, To: d.URL},
			audit.Change{Field: "run_as", Classification: audit.Open, To: d.RunAs.String()},
			audit.Change{Field: "enabled", Classification: audit.Open, To: strconv.FormatBool(d.Enabled)},
		)
	})
	if err != nil {
		return nil, err
	}
	if err := l.seed(ctx, d); err != nil {
		return nil, err
	}
	return LdapDirectoryOutput(d, true), nil
}

// checkRunAs holds the service account to what a run needs: a service account of this workspace,
// switched on, that may manage its members.
func (l LdapDirectories) checkRunAs(ctx context.Context, actor appshared.ActorContext, id shared.ID) error {
	var account domain.Account
	err := l.Directory.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		account, err = l.Directory.Accounts.Find(ctx, id)
		return err
	})
	if errors.Is(err, shared.ErrNotFound) || (err == nil && (account.Kind != domain.AccountServiceAccount || account.Status != domain.AccountActive)) {
		return ldapRefusal("directory.ldap_run_as_invalid", "/run_as")
	}
	if err != nil {
		return err
	}
	if err := l.Directory.authorize(ctx, runAs(actor.TenantID, id), AccountProvisionedAction, membersWrite, accountTarget); err != nil {
		return ldapRefusal("directory.ldap_run_as_refused", "/run_as")
	}
	return nil
}

func ldapRefusal(code, path string) error {
	return shared.ErrValidation.WithDetail(code).WithFields(shared.FieldError{Path: path, Code: code})
}

func (h RemoveLdapDirectory) invoke(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	l := h.Directories
	if err := l.authorize(ctx, actor, LdapRemovedAction, identityProviderManage); err != nil {
		return nil, err
	}
	if err := l.proveChange(ctx, actor, in.String("step_up_token")); err != nil {
		return nil, err
	}
	err := l.Directory.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := l.Store.Delete(ctx); err != nil {
			return err
		}
		return l.record(ctx, actor, LdapRemovedAction)
	})
	if err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}

func (h SyncLdapDirectory) invoke(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	l := h.Directories
	if err := l.authorize(ctx, actor, LdapSyncedAction, membersWrite); err != nil {
		return nil, err
	}
	var confirm *int
	if in.Present("confirm_switch_off") {
		given := in.Int("confirm_switch_off")
		if given < 0 {
			return nil, ldapRefusal("directory.ldap_confirm_invalid", "/confirm_switch_off")
		}
		// Letting the guard through is the one act here that can switch off many people at once.
		if err := l.proveChange(ctx, actor, in.String("step_up_token")); err != nil {
			return nil, err
		}
		confirm = &given
	}
	dry := in.Bool("dry_run")
	d, sealed, err := l.find(ctx, actor.PersistenceScope())
	if err != nil {
		return nil, err
	}
	run := l.run(ctx, d, sealed, dry, confirm)
	if !dry {
		ran := d.Ran(l.Directory.Clock.Now(), run.outcome, run.code, run.counts)
		if err := l.Directory.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
			if err := l.Store.RecordRun(ctx, ran); err != nil {
				return err
			}
			return l.record(ctx, actor, LdapSyncedAction, runChanges(run)...)
		}); err != nil {
			return nil, err
		}
	}
	return runOutput(run, dry), nil
}

// Run is the scheduled run, as the configured service account: it answers when it comes back.
// Only a failure of this installation's own (the database) is an error for the queue to retry; a
// directory that refuses, is down or answers wrong is the run's outcome, recorded, and the job comes
// back after the backoff - it never stops on its own while the directory is configured and on.
func (j LdapSyncJob) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	l := j.Directories
	if job.TenantID.IsZero() {
		return queue.Result{Outcome: "malformed"}, nil
	}
	scope := persistence.Scope{TenantID: job.TenantID}
	d, sealed, err := l.find(ctx, scope)
	if errors.Is(err, shared.ErrNotFound) {
		return queue.Result{Outcome: "removed"}, nil
	}
	if err != nil {
		return queue.Result{}, err
	}
	if !d.Enabled || d.NextRunAt == nil {
		return queue.Result{Outcome: "disabled"}, nil
	}
	now := l.Directory.Clock.Now()
	if !d.Due(now) {
		return queue.Result{Repeat: true, RepeatAfter: d.NextRunAt.Sub(now), Outcome: "waiting"}, nil
	}
	run := l.run(ctx, d, sealed, false, nil)
	ran := d.Ran(l.Directory.Clock.Now(), run.outcome, run.code, run.counts)
	err = l.Directory.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		if err := l.Store.RecordRun(ctx, ran); err != nil {
			return err
		}
		return l.record(ctx, runAs(job.TenantID, d.RunAs), LdapSyncedAction, runChanges(run)...)
	})
	if errors.Is(err, shared.ErrNotFound) {
		return queue.Result{Outcome: "removed"}, nil
	}
	if err != nil {
		return queue.Result{}, err
	}
	return queue.Result{
		Repeat: true, RepeatAfter: max(time.Minute, ran.NextRunAt.Sub(l.Directory.Clock.Now())),
		Outcome: "directory." + strings.ToLower(string(run.outcome)), Counts: countsMap(run.counts),
	}, nil
}

var _ queue.Detached = LdapSyncJob{}

// OwnsItsTransactions: a run reads a directory over the network before it writes, and no
// transaction stays open across that; each act commits in its own.
func (LdapSyncJob) OwnsItsTransactions() {}

// ldapRun is how one run went.
type ldapRun struct {
	outcome        domain.LdapOutcome
	code           string
	counts         domain.LdapCounts
	switchOff      int
	switchOffLimit int
}

func failedRun(err error) ldapRun {
	return ldapRun{outcome: domain.LdapFailed, code: codeOfLdap(err, "directory.ldap_unreachable")}
}

// run reads, plans and - unless dry - applies. It never answers an error: whatever stopped it is
// the run's outcome, so the schedule records it and comes back.
func (l LdapDirectories) run(
	ctx context.Context, d domain.LdapDirectory, sealed cryptoport.Sealed, dry bool, confirm *int,
) ldapRun {
	ctx, cancel := context.WithTimeout(ctx, ldapRunTimeout)
	defer cancel()
	actor := runAs(d.TenantID, d.RunAs)
	if err := l.Directory.authorize(ctx, actor, AccountProvisionedAction, membersWrite, accountTarget); err != nil {
		return ldapRun{outcome: domain.LdapFailed, code: "directory.ldap_run_as_refused"}
	}
	password, err := l.Encryptor.Open(ctx, sealed, LdapPasswordPurpose(d.TenantID))
	if err != nil {
		return failedRun(err)
	}
	answer, err := l.Reader.Read(ctx, ldapport.Connection{Directory: d, Password: password})
	if err != nil {
		return failedRun(err)
	}
	for i := range answer.People {
		if answer.People[i].Email != "" {
			answer.People[i].Email = domain.LookupAddress(answer.People[i].Email, l.Directory.Domains)
		}
	}
	input, err := l.planInput(ctx, actor, d, answer)
	if err != nil {
		return failedRun(err)
	}
	input.Confirm = confirm
	plan, planErr := domain.PlanLdapSync(input)
	run := ldapRun{switchOff: len(plan.SwitchOff), switchOffLimit: plan.SwitchOffLimit}
	run.counts.Seen, run.counts.Skipped = plan.Seen, plan.Skipped
	if planErr != nil {
		run.outcome, run.code = domain.LdapRefused, codeOfLdap(planErr, "directory.ldap_answer_empty")
		return run
	}
	if dry {
		run.outcome = domain.LdapOK
		run.counts.Provisioned, run.counts.Linked, run.counts.Renamed = len(plan.Provision), len(plan.Link), len(plan.Rename)
		run.counts.SwitchedOff, run.counts.SwitchedOn = len(plan.SwitchOff), len(plan.SwitchOn)
		for _, group := range plan.Groups {
			if group.GroupID.IsZero() {
				run.counts.GroupsCreated++
			}
		}
		return run
	}
	l.apply(ctx, actor, plan, &run)
	run.outcome = domain.LdapOK
	if run.counts.Failed > 0 {
		run.outcome = domain.LdapPartial
	}
	return run
}

// planInput reads whom the directory holds and the accounts under the addresses it does not.
func (l LdapDirectories) planInput(
	ctx context.Context, actor appshared.ActorContext, d domain.LdapDirectory, answer domain.LdapAnswer,
) (domain.LdapPlanInput, error) {
	input := domain.LdapPlanInput{Answer: answer, Groups: d.SyncsGroups(), ByAddress: map[string]domain.LdapAddressMatch{}}
	err := l.Directory.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		held, err := l.Store.Held(ctx)
		if err != nil {
			return err
		}
		input.Held = held
		if d.SyncsGroups() {
			if input.HeldGroups, err = l.Store.HeldGroups(ctx); err != nil {
				return err
			}
		}
		known := make(map[string]bool, len(held))
		for _, person := range held {
			known[person.ExternalID] = true
		}
		var found []shared.ID
		for _, person := range answer.People {
			if person.Email == "" || known[domain.LdapExternalID(person.ID)] {
				continue
			}
			if _, done := input.ByAddress[person.Email]; done {
				continue
			}
			account, err := l.Directory.Accounts.FindByEmail(ctx, person.Email)
			if errors.Is(err, shared.ErrNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			input.ByAddress[person.Email] = domain.LdapAddressMatch{
				AccountID: account.ID, Kind: account.Kind, Status: account.Status, Name: account.DisplayName,
			}
			found = append(found, account.ID)
		}
		if len(found) == 0 {
			return nil
		}
		external, err := l.Directory.Links.AccountExternalIDs(ctx, found)
		if err != nil {
			return err
		}
		for address, match := range input.ByAddress {
			match.ExternalID = external[match.AccountID]
			input.ByAddress[address] = match
		}
		return nil
	})
	return input, err
}

// apply carries the plan out through Directory, one act at a time: an act the workspace refuses
// is counted and the run goes on, so one person past the people limit stops nobody else.
func (l LdapDirectories) apply(ctx context.Context, actor appshared.ActorContext, plan domain.LdapPlan, run *ldapRun) {
	failed := func(err error) {
		run.counts.Failed++
		if run.code == "" {
			run.code = codeOfLdap(err, "directory.ldap_act_failed")
		}
	}
	on, off := true, false
	for _, person := range plan.Provision {
		name := person.Name
		if name == "" {
			name, _, _ = strings.Cut(person.Email, "@")
		}
		if _, err := l.Directory.ProvisionPerson(ctx, actor, ProvisionPersonCommand{
			Email: person.Email, DisplayName: name, ExternalID: domain.LdapExternalID(person.ID), Active: true,
		}); err != nil {
			failed(err)
			continue
		}
		run.counts.Provisioned++
	}
	for _, link := range plan.Link {
		external := link.ExternalID
		cmd := ChangePersonCommand{AccountID: link.AccountID, ExternalID: &external}
		if link.Name != "" {
			name := link.Name
			cmd.DisplayName = &name
		}
		if link.SwitchOn {
			cmd.Active = &on
		}
		if _, err := l.Directory.ChangePerson(ctx, actor, cmd); err != nil {
			failed(err)
			continue
		}
		run.counts.Linked++
	}
	for _, rename := range plan.Rename {
		name := rename.Name
		if _, err := l.Directory.ChangePerson(ctx, actor, ChangePersonCommand{AccountID: rename.AccountID, DisplayName: &name}); err != nil {
			failed(err)
			continue
		}
		run.counts.Renamed++
	}
	for _, id := range plan.SwitchOn {
		if _, err := l.Directory.ChangePerson(ctx, actor, ChangePersonCommand{AccountID: id, Active: &on}); err != nil {
			failed(err)
			continue
		}
		run.counts.SwitchedOn++
	}
	for _, id := range plan.SwitchOff {
		_, err := l.Directory.ChangePerson(ctx, actor, ChangePersonCommand{AccountID: id, Active: &off})
		switch code := codeOfLdap(err, ""); {
		case err == nil:
			run.counts.SwitchedOff++
		case code == "directory.owner_kept" || code == "directory.restricted_kept":
			run.counts.Kept++
		default:
			failed(err)
		}
	}
	if len(plan.Groups) == 0 && len(plan.Emptied) == 0 {
		return
	}
	accounts, err := l.heldAccounts(ctx, actor)
	if err != nil {
		failed(err)
		return
	}
	for _, group := range plan.Groups {
		members := make([]shared.ID, 0, len(group.Members))
		for _, external := range group.Members {
			if id, ok := accounts[external]; ok {
				members = append(members, id)
			}
		}
		if group.GroupID.IsZero() {
			if _, err := l.Directory.ProvisionGroup(ctx, actor, group.Name, group.ExternalID, members); err != nil {
				failed(err)
				continue
			}
			run.counts.GroupsCreated++
			continue
		}
		changed, err := l.changeGroup(ctx, actor, group.GroupID, group.Name, members)
		if err != nil {
			failed(err)
			continue
		}
		if changed {
			run.counts.GroupsChanged++
		}
	}
	for _, id := range plan.Emptied {
		changed, err := l.changeGroup(ctx, actor, id, "", []shared.ID{})
		if err != nil {
			failed(err)
			continue
		}
		if changed {
			run.counts.GroupsChanged++
		}
	}
}

// changeGroup writes a group's name and members only where they differ, so that a run that
// changes nothing writes nothing.
func (l LdapDirectories) changeGroup(
	ctx context.Context, actor appshared.ActorContext, id shared.ID, name string, members []shared.ID,
) (bool, error) {
	current, err := l.Directory.Group(ctx, actor, id, true)
	if err != nil {
		return false, err
	}
	slices.Sort(members)
	cmd := ChangeGroupCommand{GroupID: id}
	if name != "" && name != current.Group.Name {
		cmd.Name = &name
	}
	if !slices.Equal(members, current.Members) {
		cmd.Replace = &members
	}
	if cmd.Name == nil && cmd.Replace == nil {
		return false, nil
	}
	_, err = l.Directory.ChangeGroup(ctx, actor, cmd)
	return err == nil, err
}

// heldAccounts maps the directory's external identifiers to accounts after this run's people
// changes, those it just provisioned among them.
func (l LdapDirectories) heldAccounts(ctx context.Context, actor appshared.ActorContext) (map[string]shared.ID, error) {
	out := map[string]shared.ID{}
	err := l.Directory.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		held, err := l.Store.Held(ctx)
		for _, person := range held {
			if person.Status != domain.AccountDisabled {
				out[person.ExternalID] = person.AccountID
			}
		}
		return err
	})
	return out, err
}

func (l LdapDirectories) record(ctx context.Context, actor appshared.ActorContext, action audit.Action, changes ...audit.Change) error {
	return l.Directory.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: l.Directory.Clock.Now(), Action: action,
		Outcome: audit.OutcomeSuccess, Severity: audit.SeverityNotice,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: ldapTarget, TargetID: actor.TenantID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(changes...),
	})
}

func runChanges(run ldapRun) []audit.Change {
	changes := []audit.Change{{Field: "outcome", Classification: audit.Open, To: string(run.outcome)}}
	if run.code != "" {
		changes = append(changes, audit.Change{Field: "error", Classification: audit.Open, To: run.code})
	}
	for _, field := range []struct {
		name  string
		count int
	}{
		{"provisioned", run.counts.Provisioned}, {"switched_off", run.counts.SwitchedOff},
		{"switched_on", run.counts.SwitchedOn}, {"failed", run.counts.Failed},
	} {
		changes = append(changes, audit.Change{Field: field.name, Classification: audit.Open, To: strconv.Itoa(field.count)})
	}
	return changes
}

// codeOfLdap is an error as a message code, never a server's words; fallback for one without.
func codeOfLdap(err error, fallback string) string {
	var coded *shared.Error
	if errors.As(err, &coded) && coded.DetailCode != "" {
		return coded.DetailCode
	}
	return fallback
}

func countsOutput(c domain.LdapCounts) usecase.Output {
	return usecase.Output{
		"seen": c.Seen, "provisioned": c.Provisioned, "linked": c.Linked, "renamed": c.Renamed,
		"switched_off": c.SwitchedOff, "switched_on": c.SwitchedOn, "kept": c.Kept, "skipped": c.Skipped,
		"failed": c.Failed, "groups_created": c.GroupsCreated, "groups_changed": c.GroupsChanged,
	}
}

func countsMap(c domain.LdapCounts) map[string]int {
	out := map[string]int{}
	for name, value := range countsOutput(c) {
		out[name], _ = value.(int)
	}
	return out
}

func runOutput(run ldapRun, dry bool) usecase.Output {
	out := usecase.Output{
		"outcome": string(run.outcome), "dry_run": dry, "counts": countsOutput(run.counts),
		"switch_off_planned": run.switchOff, "switch_off_limit": run.switchOffLimit,
	}
	if run.code != "" {
		out["error"] = run.code
	}
	return out
}

// LdapDirectoryOutput is the directory as every door answers it (schema LdapDirectory), never its
// password.
func LdapDirectoryOutput(d domain.LdapDirectory, hasPassword bool) usecase.Output {
	out := usecase.Output{
		"url": d.URL, "bind_dn": d.BindDN, "has_password": hasPassword, "user_base": d.UserBase,
		"user_filter": d.UserFilter, "group_filter": d.GroupFilter, "id_attribute": d.IDAttribute,
		"mail_attribute": d.MailAttribute, "name_attribute": d.NameAttribute,
		"group_name_attribute": d.GroupNameAttribute, "member_attribute": d.MemberAttribute,
		"run_as": d.RunAs.String(), "every_minutes": d.EveryMinutes, "enabled": d.Enabled,
		"failures": d.Failures, "created_at": d.CreatedAt, "updated_at": d.UpdatedAt,
	}
	if d.GroupBase != "" {
		out["group_base"] = d.GroupBase
	}
	if d.NextRunAt != nil {
		out["next_run_at"] = *d.NextRunAt
	}
	if d.LastRunAt != nil {
		out["last_run_at"] = *d.LastRunAt
	}
	if d.LastOutcome != "" {
		out["last_outcome"] = string(d.LastOutcome)
	}
	if d.LastError != "" {
		out["last_error"] = d.LastError
	}
	if d.LastCounts != nil {
		out["last_counts"] = countsOutput(*d.LastCounts)
	}
	return out
}
