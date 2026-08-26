// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"context"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domainservice "github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

var (
	tenantID = shared.MustParseID("0192f000-0000-7000-8000-00000000000a")
	actorID  = shared.MustParseID("0192f000-0000-7000-8000-00000000000b")
	targetID = shared.MustParseID("0192f000-0000-7000-8000-00000000000c")
	now      = time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC)
)

func caller() appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantID, AccountID: actorID,
		AccountName: "Ada", Locale: "en", TimeZone: "UTC",
	}
}

// The doubles. Each one is the shape of the port and nothing more; what is being tested here is
// which questions the use cases ask and in which order, not what a bucket does.

type targetStore struct {
	stored     []domain.Target
	credential crypto.Sealed
	tested     []string
	failFind   bool
}

func (s *targetStore) Insert(_ context.Context, target domain.Target, credential crypto.Sealed) error {
	s.stored = append(s.stored, target)
	s.credential = credential
	return nil
}

func (s *targetStore) List(context.Context) ([]domain.Target, error) { return s.stored, nil }

func (s *targetStore) Find(_ context.Context, id shared.ID) (domain.Target, error) {
	if s.failFind {
		return domain.Target{}, shared.ErrNotFound.WithDetail("backup.target_not_found")
	}
	for _, target := range s.stored {
		if target.ID == id {
			return target, nil
		}
	}
	return domain.Target{}, shared.ErrNotFound.WithDetail("backup.target_not_found")
}

func (s *targetStore) Credential(context.Context, shared.ID) (crypto.Sealed, error) {
	return s.credential, nil
}

func (s *targetStore) RecordTest(_ context.Context, _ shared.ID, _ time.Time, ok bool, code string) error {
	s.tested = append(s.tested, map[bool]string{true: "ok", false: "failed:" + code}[ok])
	return nil
}

func (s *targetStore) Coverage(context.Context) (repository.Coverage, error) {
	return repository.Coverage{Configured: len(s.stored)}, nil
}

// memoryStore is a target that keeps its objects in a map: enough for the probe to succeed, and
// small enough that the failures it is made to produce are obvious.
type memoryStore struct {
	objects    map[string][]byte
	failPut    bool
	otherBytes bool
	deletes    int
}

func (s *memoryStore) Put(_ context.Context, key string, content io.Reader) (int64, error) {
	if s.failPut {
		return 0, shared.ErrUnavailable.WithDetail(backupstorage.CodeTargetRefused)
	}
	body, err := io.ReadAll(content)
	if err != nil {
		return 0, err
	}
	if s.otherBytes {
		body = []byte("something else entirely")
	}
	s.objects[key] = body
	return int64(len(body)), nil
}

func (s *memoryStore) Get(_ context.Context, key string) (io.ReadCloser, error) {
	body, found := s.objects[key]
	if !found {
		return nil, shared.ErrNotFound.WithDetail(backupstorage.CodeObjectNotFound)
	}
	return io.NopCloser(strings.NewReader(string(body))), nil
}

// List and Stat are real rather than empty, because a backup run reads a target back: the archive
// reader lists to find the manifests, and a retention pass lists to find what it may delete.
func (s *memoryStore) List(_ context.Context, prefix string) ([]backupstorage.Entry, error) {
	var entries []backupstorage.Entry
	for key, body := range s.objects {
		if strings.HasPrefix(key, prefix) {
			entries = append(entries, backupstorage.Entry{Key: key, Size: int64(len(body))})
		}
	}
	slices.SortFunc(entries, func(a, b backupstorage.Entry) int { return strings.Compare(a.Key, b.Key) })
	return entries, nil
}

func (s *memoryStore) Stat(_ context.Context, key string) (backupstorage.Entry, error) {
	body, found := s.objects[key]
	if !found {
		return backupstorage.Entry{}, shared.ErrNotFound.WithDetail(backupstorage.CodeObjectNotFound)
	}
	return backupstorage.Entry{Key: key, Size: int64(len(body))}, nil
}

func (s *memoryStore) Delete(_ context.Context, key string) error {
	s.deletes++
	delete(s.objects, key)
	return nil
}

type opener struct {
	store   *memoryStore
	kinds   []domain.TargetKind
	failure error
	opened  []backupstorage.Spec
}

func (o *opener) Open(_ context.Context, spec backupstorage.Spec) (backupstorage.Store, error) {
	o.opened = append(o.opened, spec)
	if o.failure != nil {
		return nil, o.failure
	}
	return o.store, nil
}

func (o *opener) Kinds() []domain.TargetKind { return o.kinds }

// encryptor keeps the plaintexts it was given and hands back an opaque handle, so that a test can
// tell "this was sealed" from "this was stored", and so that a ciphertext containing the plaintext
// would be a visible failure rather than the fake's own doing.
type encryptor struct {
	sealed   []string
	purposes []crypto.Purpose
	noKey    bool
}

func (e *encryptor) Seal(_ context.Context, plaintext secret.Secret, purpose crypto.Purpose) (crypto.Sealed, error) {
	if e.noKey {
		return crypto.Sealed{}, shared.ErrUnavailable.WithDetail(crypto.CodeNoEncryptionKey)
	}
	e.sealed = append(e.sealed, plaintext.Reveal())
	e.purposes = append(e.purposes, purpose)
	return crypto.Sealed{
		KeyID: "k1", Ciphertext: []byte("opaque#" + strconv.Itoa(len(e.sealed)-1)),
	}, nil
}

func (e *encryptor) Open(_ context.Context, sealed crypto.Sealed, purpose crypto.Purpose) (secret.Secret, error) {
	e.purposes = append(e.purposes, purpose)
	index, err := strconv.Atoi(strings.TrimPrefix(string(sealed.Ciphertext), "opaque#"))
	if err != nil || index >= len(e.sealed) {
		return secret.Secret{}, crypto.NotAuthentic()
	}
	return secret.New(e.sealed[index]), nil
}

func (e *encryptor) ActiveKeyID() string { return "k1" }

type authorizerDouble struct {
	err      error
	requests []access.Request
}

func (a *authorizerDouble) Authorize(_ context.Context, _ appshared.ActorContext, request access.Request) error {
	a.requests = append(a.requests, request)
	return a.err
}

type unitOfWork struct{ writes, reads int }

func (u *unitOfWork) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	u.writes++
	return fn(ctx)
}

func (u *unitOfWork) WithinReadOnly(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	u.reads++
	return fn(ctx)
}

type sink struct{ entries []audit.Entry }

func (s *sink) Append(_ context.Context, entry audit.Entry) error {
	if err := entry.Validate(); err != nil {
		return err
	}
	s.entries = append(s.entries, entry)
	return nil
}

type ids struct{ next shared.ID }

func (i ids) NewID() shared.ID { return i.next }

type harness struct {
	targets    *targetStore
	opener     *opener
	encryptor  *encryptor
	authorizer *authorizerDouble
	audit      *sink
	uow        *unitOfWork
	config     env.Config
}

func newHarness() *harness {
	return &harness{
		targets: &targetStore{},
		opener: &opener{
			store: &memoryStore{objects: map[string][]byte{}},
			kinds: []domain.TargetKind{domain.KindLocal, domain.KindS3, domain.KindWebDAV},
		},
		encryptor:  &encryptor{},
		authorizer: &authorizerDouble{},
		audit:      &sink{},
		uow:        &unitOfWork{},
		config:     env.Config{Tenancy: env.TenancySingle},
	}
}

func (h *harness) writer() Writer {
	return Writer{
		Targets: h.targets, Opener: h.opener, Encryptor: h.encryptor,
		Authorizer: h.authorizer, Audit: h.audit, UnitOfWork: h.uow,
		Clock: clock.Fixed(now), IDs: ids{next: targetID}, Config: h.config,
	}
}

func command() CreateBackupTargetCommand {
	return CreateBackupTargetCommand{
		Name: "Off-site bucket", Kind: domain.KindS3,
		Config: domain.TargetConfig{"bucket": "hubtask-backups", "endpoint": "https://s3.example.org"},
		Credentials: map[string]secret.Secret{
			"access_key": secret.New("AKIAEXAMPLE"),
			"secret_key": secret.New("the-secret-access-key"),
		},
	}
}

func TestCreatingATargetSealsItsCredentialAndAsksForTheOwnersRight(t *testing.T) {
	h := newHarness()

	target, err := (CreateBackupTarget{Writer: h.writer()}).
		Execute(context.Background(), caller(), command())
	if err != nil {
		t.Fatalf("creating: %v", err)
	}
	if target.ID != targetID || target.TenantID != tenantID {
		t.Fatalf("the target is %s in %s", target.ID, target.TenantID)
	}

	request := h.authorizer.requests[0]
	switch {
	case request.Permission != domainservice.PermissionDeleteContainer:
		t.Errorf("asked for %q - a target is a channel the tenant's data leaves by", request.Permission)
	case request.TokenScope != backupManage:
		t.Errorf("scope %q", request.TokenScope)
	}

	// The credential is sealed, and the ciphertext is what was stored.
	if len(h.encryptor.sealed) != 1 {
		t.Fatalf("%d credentials sealed", len(h.encryptor.sealed))
	}
	if !strings.Contains(h.encryptor.sealed[0], "the-secret-access-key") {
		t.Fatal("something other than the credential was sealed")
	}
	if strings.Contains(string(h.targets.credential.Ciphertext), "the-secret-access-key") {
		t.Fatal("the credential was stored in the clear")
	}
	if h.targets.credential.KeyID != "k1" {
		t.Fatalf("the stored credential names key %q", h.targets.credential.KeyID)
	}
	// Bound to the row, so a ciphertext lifted out of one target does not open in another.
	if string(h.encryptor.purposes[0]) != "backup_target.credential:"+targetID.String() {
		t.Fatalf("sealed under the purpose %q", h.encryptor.purposes[0])
	}
}

// The target is opened before it is stored: a configuration that cannot become a connection is a
// field error now rather than a failed backup at three in the morning.
func TestATargetThatCannotBecomeAConnectionIsRefusedBeforeItIsStored(t *testing.T) {
	h := newHarness()
	h.opener.failure = shared.ErrValidation.WithDetail("backup.url_invalid")

	_, err := (CreateBackupTarget{Writer: h.writer()}).
		Execute(context.Background(), caller(), command())
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("creating: %v", err)
	}
	if len(h.targets.stored) != 0 {
		t.Fatal("the target was stored anyway")
	}
	if len(h.audit.entries) != 0 {
		t.Fatal("an audit entry was written for a target that was not created")
	}
}

func TestAKindThisBuildCannotTalkToIsRefusedByName(t *testing.T) {
	h := newHarness()
	cmd := command()
	cmd.Kind, cmd.Config = domain.KindSMB, domain.TargetConfig{"host": "nas", "share": "backups"}

	_, err := (CreateBackupTarget{Writer: h.writer()}).Execute(context.Background(), caller(), cmd)
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("creating: %v", err)
	}
	if code := shared.AsError(err).DetailCode; code != backupstorage.CodeKindUnsupported {
		t.Fatalf("detail code %q", code)
	}
	// The refusal names the kind, so a client can say which one rather than "unsupported".
	if shared.AsError(err).Params["kind"] != "SMB" {
		t.Fatalf("the refusal names %q", shared.AsError(err).Params["kind"])
	}
}

// The switch backup-restore.md §2 names. It has no meaning in single-tenant operation, where the
// tenant's owner is the person running the installation.
func TestATenantMayNotConfigureATargetUnlessTheOperatorSaysSo(t *testing.T) {
	h := newHarness()
	h.config = env.Config{Tenancy: env.TenancyMulti}

	_, err := (CreateBackupTarget{Writer: h.writer()}).
		Execute(context.Background(), caller(), command())
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("creating in provider operation: %v", err)
	}
	if code := shared.AsError(err).DetailCode; code != "backup.tenant_targets_disabled" {
		t.Fatalf("detail code %q", code)
	}

	h.config.Backup.TenantTargets = true
	if _, err := (CreateBackupTarget{Writer: h.writer()}).
		Execute(context.Background(), caller(), command()); err != nil {
		t.Fatalf("creating with the switch on: %v", err)
	}

	// And in single-tenant operation the switch is not consulted at all.
	single := newHarness()
	if _, err := (CreateBackupTarget{Writer: single.writer()}).
		Execute(context.Background(), caller(), command()); err != nil {
		t.Fatalf("creating in single-tenant operation: %v", err)
	}
}

// An installation with no encryption key refuses to store a credential rather than writing one in
// the clear (E-02).
func TestATargetWithACredentialNeedsAKeyToSealItWith(t *testing.T) {
	h := newHarness()
	h.encryptor.noKey = true

	_, err := (CreateBackupTarget{Writer: h.writer()}).
		Execute(context.Background(), caller(), command())
	if !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("creating: %v", err)
	}
	if len(h.targets.stored) != 0 {
		t.Fatal("the target was stored without its credential")
	}

	// A target that needs no credential is unaffected: it stores none, and no key identifier
	// either, which is honest where a sealed empty map would claim there is something to open.
	cmd := command()
	cmd.Kind, cmd.Config, cmd.Credentials = domain.KindLocal, domain.TargetConfig{"path": "backups"}, nil
	if _, err := (CreateBackupTarget{Writer: h.writer()}).
		Execute(context.Background(), caller(), cmd); err != nil {
		t.Fatalf("creating a target with no credential: %v", err)
	}
	if !h.targets.credential.IsZero() {
		t.Fatalf("a target with no credential stored %v", h.targets.credential)
	}
}

// The entry backup-restore.md §2 asks for: where the data may now go, recorded openly, and not a
// word of what it authenticates with.
func TestCreatingATargetIsInTheTrailAndTheCredentialIsNot(t *testing.T) {
	h := newHarness()

	if _, err := (CreateBackupTarget{Writer: h.writer()}).
		Execute(context.Background(), caller(), command()); err != nil {
		t.Fatalf("creating: %v", err)
	}
	if len(h.audit.entries) != 1 {
		t.Fatalf("%d audit entries", len(h.audit.entries))
	}

	entry := h.audit.entries[0]
	if entry.Action != TargetChangedAction || entry.Severity != audit.SeverityWarning {
		t.Fatalf("the entry is %q at %q", entry.Action, entry.Severity)
	}

	var recorded []string
	for field, change := range entry.Changes {
		recorded = append(recorded, field+"="+valueOf(change))
	}
	joined := strings.Join(recorded, " ")
	if !strings.Contains(joined, "config.bucket=hubtask-backups") {
		t.Fatalf("the trail does not say where the data goes: %s", joined)
	}
	for _, secretValue := range []string{"AKIAEXAMPLE", "the-secret-access-key"} {
		if strings.Contains(joined, secretValue) {
			t.Fatalf("the trail quoted a credential: %s", joined)
		}
	}
}

// An acknowledged insecure target records who accepted it, because "somebody ticked a box" is not
// an answer to "who decided this".
func TestAnAcknowledgedTargetRecordsWhatWasAccepted(t *testing.T) {
	h := newHarness()
	cmd := command()
	cmd.EncryptionMode, cmd.InsecureAcknowledged = domain.EncryptionNone, true

	target, err := (CreateBackupTarget{Writer: h.writer()}).Execute(context.Background(), caller(), cmd)
	if err != nil {
		t.Fatalf("creating: %v", err)
	}
	if target.InsecureAckBy != actorID || !target.InsecureAckAt.Equal(now) {
		t.Fatalf("acknowledged by %s at %s", target.InsecureAckBy, target.InsecureAckAt)
	}

	accepted := valueOf(h.audit.entries[0].Changes["insecure_acknowledged"])
	if !strings.Contains(accepted, domain.WarningUnencrypted) {
		t.Fatalf("the trail records the acknowledgement as %q", accepted)
	}
}

func TestListingAsksForTheAdministratorsLineAndReadsOnly(t *testing.T) {
	h := newHarness()
	if _, err := (CreateBackupTarget{Writer: h.writer()}).
		Execute(context.Background(), caller(), command()); err != nil {
		t.Fatalf("creating: %v", err)
	}

	before := h.uow.writes
	targets, err := (ListBackupTargets{Writer: h.writer()}).Execute(context.Background(), caller())
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("%d targets", len(targets))
	}
	if h.uow.writes != before {
		t.Fatal("a listing opened a write transaction")
	}

	request := h.authorizer.requests[len(h.authorizer.requests)-1]
	if request.Permission != domainservice.PermissionStructure {
		t.Errorf("asked for %q", request.Permission)
	}
	if request.TokenScope != backupRead {
		t.Errorf("scope %q - reading which targets exist is its own scope", request.TokenScope)
	}
}

func TestTheProbeWritesReadsBackAndRemovesWhatItWrote(t *testing.T) {
	h := newHarness()
	if _, err := (CreateBackupTarget{Writer: h.writer()}).
		Execute(context.Background(), caller(), command()); err != nil {
		t.Fatalf("creating: %v", err)
	}

	probe, err := (TestBackupTarget{Writer: h.writer()}).
		Execute(context.Background(), caller(), targetID)
	if err != nil {
		t.Fatalf("probing: %v", err)
	}
	switch {
	case !probe.OK:
		t.Fatalf("the probe failed: %q", probe.ErrorCode)
	case !probe.Writable:
		t.Fatal("the probe says the target is not writable")
	}
	if len(h.opener.store.objects) != 0 {
		t.Fatalf("the probe left %v behind", h.opener.store.objects)
	}
	if h.opener.store.deletes != 1 {
		t.Fatalf("%d deletions", h.opener.store.deletes)
	}
	if len(h.targets.tested) != 1 || h.targets.tested[0] != "ok" {
		t.Fatalf("the result was written down as %v", h.targets.tested)
	}
}

// A target that is unreachable is a result rather than an error: it is what the caller asked to
// find out, and a 502 would say this server failed instead.
func TestAnUnreachableTargetIsAResultAndIsWrittenDown(t *testing.T) {
	h := newHarness()
	if _, err := (CreateBackupTarget{Writer: h.writer()}).
		Execute(context.Background(), caller(), command()); err != nil {
		t.Fatalf("creating: %v", err)
	}
	h.opener.store.failPut = true

	probe, err := (TestBackupTarget{Writer: h.writer()}).
		Execute(context.Background(), caller(), targetID)
	if err != nil {
		t.Fatalf("probing an unreachable target reported an error: %v", err)
	}
	switch {
	case probe.OK:
		t.Fatal("the probe says it worked")
	case probe.Writable:
		t.Fatal("the probe says the target is writable")
	case probe.ErrorCode != backupstorage.CodeTargetRefused:
		t.Fatalf("the reason is %q", probe.ErrorCode)
	}
	if len(h.targets.tested) != 1 || !strings.HasPrefix(h.targets.tested[0], "failed:") {
		t.Fatalf("the result was written down as %v", h.targets.tested)
	}
	if h.audit.entries[len(h.audit.entries)-1].Outcome != audit.OutcomeFailed {
		t.Fatal("the trail records a failed probe as a success")
	}
}

// The failure a status code cannot describe. A target that accepts a write and gives back
// something else is worse than one that refuses, because a backup written to it looks like one.
func TestATargetThatGivesBackOtherBytesFailsTheProbe(t *testing.T) {
	h := newHarness()
	if _, err := (CreateBackupTarget{Writer: h.writer()}).
		Execute(context.Background(), caller(), command()); err != nil {
		t.Fatalf("creating: %v", err)
	}
	h.opener.store.otherBytes = true

	probe, err := (TestBackupTarget{Writer: h.writer()}).
		Execute(context.Background(), caller(), targetID)
	if err != nil {
		t.Fatalf("probing: %v", err)
	}
	if probe.OK || probe.ErrorCode != "backup.target_returned_other_bytes" {
		t.Fatalf("the probe says %v / %q", probe.OK, probe.ErrorCode)
	}
}

func TestProbingWithoutATargetIsAValidationError(t *testing.T) {
	h := newHarness()

	_, err := (TestBackupTarget{Writer: h.writer()}).Execute(context.Background(), caller(), "")
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("probing nothing: %v", err)
	}
	if len(h.authorizer.requests) != 0 {
		t.Fatal("a request with no target reached the authorisation service")
	}
}

func TestARefusedRequestNeverReachesTheTarget(t *testing.T) {
	h := newHarness()
	h.authorizer.err = shared.ErrForbidden.WithDetail("access.not_permitted")

	if _, err := (CreateBackupTarget{Writer: h.writer()}).
		Execute(context.Background(), caller(), command()); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("creating: %v", err)
	}
	if _, err := (ListBackupTargets{Writer: h.writer()}).
		Execute(context.Background(), caller()); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("listing: %v", err)
	}
	if _, err := (TestBackupTarget{Writer: h.writer()}).
		Execute(context.Background(), caller(), targetID); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("probing: %v", err)
	}
	if len(h.opener.opened) != 0 {
		t.Fatal("a refused request opened a connection to the target")
	}
	if h.uow.reads != 0 || h.uow.writes != 0 {
		t.Fatal("a refused request opened a transaction")
	}
}

// What the three channels are built from, and what gate SG-13 reads.
func TestTheCatalogueEntriesSayWhatTheChannelsNeed(t *testing.T) {
	h := newHarness()
	create := (CreateBackupTarget{Writer: h.writer()}).Descriptor()
	list := (ListBackupTargets{Writer: h.writer()}).Descriptor()
	probe := (TestBackupTarget{Writer: h.writer()}).Descriptor()

	switch {
	case create.RESTOperation() != "createBackupTarget" || create.MCPTool() != "create_backup_target":
		t.Errorf("channel identities %q and %q", create.RESTOperation(), create.MCPTool())
	case !create.Audit.Required || create.Audit.Severity != audit.SeverityWarning:
		t.Error("creating an egress channel is not declared as something the trail owes a warning")
	case !list.ReadOnly:
		t.Error("the listing is not marked read-only")
	case list.TokenScope == create.TokenScope:
		t.Error("reading which targets exist and creating one are the same scope")
	case !probe.Audit.Required:
		t.Error("the probe writes outside this system and declares no audit obligation")
	}

	var fields []string
	for _, field := range create.Input {
		fields = append(fields, field.Name)
	}
	for _, wanted := range []string{"name", "kind", "config", "credentials", "insecure_acknowledged"} {
		if !slices.Contains(fields, wanted) {
			t.Errorf("the creation does not declare %s: %v", wanted, fields)
		}
	}
}

// The narrowing of the contract's `additionalProperties: true`. A client that sends a number means
// the same thing as one that sends a string, and a nested document is dropped rather than
// stringified into "map[a:1]".
func TestAConfigurationIsFlattenedToScalars(t *testing.T) {
	h := newHarness()

	out, err := (CreateBackupTarget{Writer: h.writer()}).invoke(
		context.Background(), caller(), usecase.Input{
			"name": "The NAS", "kind": string(domain.KindWebDAV),
			"config": map[string]any{
				"url":     "https://nas.example.org/backups",
				"port":    float64(8443),
				"verify":  true,
				"nested":  map[string]any{"no": "thanks"},
				"padding": "  trimmed  ",
			},
			"credentials": map[string]any{"password": "hunter2", "empty": ""},
		})
	if err != nil {
		t.Fatalf("creating: %v", err)
	}

	stored := h.targets.stored[0]
	switch {
	case stored.Config.Get("port") != "8443":
		t.Errorf("a number came through as %q", stored.Config.Get("port"))
	case stored.Config.Get("verify") != "true":
		t.Errorf("a flag came through as %q", stored.Config.Get("verify"))
	case stored.Config.Get("padding") != "trimmed":
		t.Errorf("whitespace survived: %q", stored.Config.Get("padding"))
	}
	if _, present := stored.Config["nested"]; present {
		t.Error("a nested document reached the configuration")
	}

	// The answer carries no credential and nowhere to put one.
	for field := range out {
		if field == "credentials" || field == "credential" {
			t.Fatalf("the answer carries %q", field)
		}
	}
	if !strings.Contains(h.encryptor.sealed[0], "hunter2") {
		t.Error("the password was not sealed")
	}
	if strings.Contains(h.encryptor.sealed[0], `"empty"`) {
		t.Error("an empty credential was stored as one")
	}
}

func TestTheListingAnswersRowsAndWarnings(t *testing.T) {
	h := newHarness()
	cmd := command()
	cmd.EncryptionMode, cmd.InsecureAcknowledged = domain.EncryptionNone, true
	if _, err := (CreateBackupTarget{Writer: h.writer()}).
		Execute(context.Background(), caller(), cmd); err != nil {
		t.Fatalf("creating: %v", err)
	}

	out, err := (ListBackupTargets{Writer: h.writer()}).
		invoke(context.Background(), caller(), usecase.Input{})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}

	rows, _ := out["data"].([]usecase.Output)
	if len(rows) != 1 {
		t.Fatalf("%d rows", len(rows))
	}
	warnings, _ := rows[0]["warnings"].([]string)
	if !slices.Contains(warnings, domain.WarningUnencrypted) {
		t.Fatalf("the row warns %v", warnings)
	}
	if rows[0]["scope"] != "TENANT" {
		t.Fatalf("the row is scoped %v", rows[0]["scope"])
	}
}

func TestTheProbeAnswersTheShapeTheContractDescribes(t *testing.T) {
	h := newHarness()
	if _, err := (CreateBackupTarget{Writer: h.writer()}).
		Execute(context.Background(), caller(), command()); err != nil {
		t.Fatalf("creating: %v", err)
	}

	out, err := (TestBackupTarget{Writer: h.writer()}).invoke(
		context.Background(), caller(), usecase.Input{"target_id": targetID.String()})
	if err != nil {
		t.Fatalf("probing: %v", err)
	}
	if out["ok"] != true || out["writable"] != true {
		t.Fatalf("the answer is %v", out)
	}
	// A store that cannot say how much room is left says nothing rather than zero, which is what
	// the contract's nullable field is for.
	if _, present := out["free_bytes"]; present {
		t.Fatal("a target that cannot report free space reported some")
	}
	if _, present := out["error_code"]; present {
		t.Fatal("a probe that worked carries a reason")
	}
}

// valueOf reads the "to" side of one masked change. audit.Changes answers a map of maps, because
// what a column holds is a document rather than a list.
func valueOf(change any) string {
	entry, _ := change.(map[string]any)
	value, _ := entry["to"].(string)
	return value
}
