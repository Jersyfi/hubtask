// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	backuprepo "github.com/Jersyfi/hubtask/core/application/repository/backup"
	backupservice "github.com/Jersyfi/hubtask/core/application/service/backup"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/core/port/clock"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// The installation's own backup targets (multi-tenancy.md §5, UC-BAK-07 check 1), through the
// registry: the operator's alone, behind a step-up for the writes, journalled without the
// credential, and stored in the installation's scope.

type installationTargets struct {
	rows        map[shared.ID]domain.Target
	credentials map[shared.ID]cryptoport.Sealed
	tested      []string
}

func newInstallationTargets() *installationTargets {
	return &installationTargets{rows: map[shared.ID]domain.Target{}, credentials: map[shared.ID]cryptoport.Sealed{}}
}

func (s *installationTargets) Insert(_ context.Context, target domain.Target, sealed cryptoport.Sealed) error {
	s.rows[target.ID], s.credentials[target.ID] = target, sealed
	return nil
}

func (s *installationTargets) List(context.Context) ([]domain.Target, error) {
	listed := []domain.Target{}
	for _, row := range s.rows {
		listed = append(listed, row)
	}
	return listed, nil
}

func (s *installationTargets) Find(_ context.Context, id shared.ID) (domain.Target, error) {
	row, held := s.rows[id]
	if !held {
		return domain.Target{}, shared.ErrNotFound.WithDetail("backup.target_not_found")
	}
	return row, nil
}

func (s *installationTargets) Credential(_ context.Context, id shared.ID) (cryptoport.Sealed, error) {
	return s.credentials[id], nil
}

func (s *installationTargets) RecordTest(_ context.Context, _ shared.ID, _ time.Time, ok bool, code string) error {
	s.tested = append(s.tested, map[bool]string{true: "ok", false: code}[ok])
	return nil
}

func (s *installationTargets) Coverage(context.Context) (backuprepo.Coverage, error) {
	return backuprepo.Coverage{}, nil
}

func (s *installationTargets) Delete(_ context.Context, id shared.ID) (bool, error) {
	_, held := s.rows[id]
	delete(s.rows, id)
	return held, nil
}

// bucket is a target that keeps what it is given.
type bucket struct{ objects map[string]string }

func (b *bucket) Put(_ context.Context, key string, content io.Reader) (int64, error) {
	body, err := io.ReadAll(content)
	b.objects[key] = string(body)
	return int64(len(body)), err
}

func (b *bucket) Get(_ context.Context, key string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(b.objects[key])), nil
}

func (b *bucket) List(context.Context, string) ([]backupstorage.Entry, error) { return nil, nil }

func (b *bucket) Stat(context.Context, string) (backupstorage.Entry, error) {
	return backupstorage.Entry{}, nil
}

func (b *bucket) Delete(_ context.Context, key string) error {
	delete(b.objects, key)
	return nil
}

type bucketOpener struct{ specs []backupstorage.Spec }

func (o *bucketOpener) Open(_ context.Context, spec backupstorage.Spec) (backupstorage.Store, error) {
	o.specs = append(o.specs, spec)
	return &bucket{objects: map[string]string{}}, nil
}

func (o *bucketOpener) Kinds() []domain.TargetKind { return []domain.TargetKind{domain.KindS3} }

// openingEnvelope is plainEnvelope that opens what it sealed, so the probe can read the credential.
type openingEnvelope struct{ plainEnvelope }

func (e *openingEnvelope) Open(
	_ context.Context, sealed cryptoport.Sealed, _ cryptoport.Purpose,
) (secret.Secret, error) {
	return secret.New(strings.TrimPrefix(string(sealed.Ciphertext), "sealed:")), nil
}

type targetsFixture struct {
	registry *usecase.Registry
	targets  *installationTargets
	journal  *journalStore
	work     *unitOfWork
	stepUp   *stepUpFake
	envelope *openingEnvelope
}

func newTargetsFixture(t *testing.T) targetsFixture {
	t.Helper()
	instance, _, journal := newInstanceWriter(newRegister(operatorID))
	work := &unitOfWork{}
	instance.UnitOfWork = work
	targets, envelope, stepUp := newInstallationTargets(), &openingEnvelope{}, &stepUpFake{expect: "fresh-proof"}
	writer := InstanceBackupTargetWriter{
		Instance: instance, Targets: targets, StepUp: stepUp,
		Backup: backupservice.Writer{
			Targets: targets, Opener: &bucketOpener{}, Encryptor: envelope,
			UnitOfWork: work, Clock: clock.Fixed(fixed), IDs: &ids{},
		},
	}
	registry, err := usecase.NewRegistry(nil,
		ListInstanceBackupTargets{Writer: writer}.Descriptor(),
		CreateInstanceBackupTarget{Writer: writer}.Descriptor(),
		TestInstanceBackupTarget{Writer: writer}.Descriptor(),
		DeleteInstanceBackupTarget{Writer: writer}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("not registrable: %v", err)
	}
	return targetsFixture{registry: registry, targets: targets, journal: journal, work: work, stepUp: stepUp, envelope: envelope}
}

func targetInput(proof string) usecase.Input {
	return usecase.Input{
		"name": "Exports", "kind": "S3",
		"config":        map[string]any{"bucket": "installation-exports", "endpoint": "https://s3.example.org"},
		"credentials":   map[string]any{"access_key": "AKIA", "secret_key": "the-secret-key"},
		"step_up_token": proof,
	}
}

func TestAnOperatorConfiguresTestsAndRemovesAnInstallationTarget(t *testing.T) {
	f := newTargetsFixture(t)

	created, err := f.registry.Invoke(t.Context(), CreateInstanceBackupTargetName, operator(), targetInput("fresh-proof"))
	if err != nil {
		t.Fatalf("configuring: %v", err)
	}
	id := shared.ID(created.String("id"))
	stored := f.targets.rows[id]
	if !stored.IsInstanceWide() || created.String("scope") != "INSTANCE" {
		t.Errorf("stored for a workspace: %+v", stored)
	}
	// Written in the installation's own scope, which no workspace session has.
	if last := f.work.scopes[len(f.work.scopes)-1]; last != persistence.SystemScope() {
		t.Errorf("written in %+v", last)
	}
	// Sealed for this row, never answered, never journalled.
	if f.targets.credentials[id].IsZero() || !strings.HasSuffix(string(f.envelope.purposes[0]), id.String()) {
		t.Errorf("sealed under %v", f.envelope.purposes)
	}
	answer, _ := json.Marshal(created)
	journalled, _ := json.Marshal(f.journal.entries)
	for _, leaked := range []string{"the-secret-key", "AKIA"} {
		if strings.Contains(string(answer), leaked) || strings.Contains(string(journalled), leaked) {
			t.Errorf("the credential reached the answer or the journal: %s / %s", answer, journalled)
		}
	}
	if len(f.journal.entries) != 1 || f.journal.entries[0].Action != string(instanceTargetConfiguredAction) ||
		f.journal.entries[0].Details["config.bucket"] != "installation-exports" {
		t.Errorf("journalled %+v", f.journal.entries)
	}

	probe, err := f.registry.Invoke(t.Context(), TestInstanceBackupTargetName, operator(), usecase.Input{"target_id": id.String()})
	if err != nil || probe["ok"] != true || len(f.targets.tested) != 1 {
		t.Errorf("probe %v (%v), recorded %v", probe, err, f.targets.tested)
	}

	if _, err := f.registry.Invoke(t.Context(), DeleteInstanceBackupTargetName, operator(),
		usecase.Input{"target_id": id.String(), "step_up_token": "fresh-proof"}); err != nil {
		t.Fatalf("removing: %v", err)
	}
	if len(f.targets.rows) != 0 {
		t.Error("the target is still configured")
	}
}

// Configuring and removing are privileged writes: without a fresh proof nothing is sealed, stored
// or journalled.
func TestTheWritesDemandAFreshStepUp(t *testing.T) {
	f := newTargetsFixture(t)

	_, err := f.registry.Invoke(t.Context(), CreateInstanceBackupTargetName, operator(), targetInput(""))
	if code := shared.AsError(err); code == nil || code.DetailCode != "auth.step_up_required" {
		t.Fatalf("without a proof: %v", err)
	}
	if len(f.targets.rows) != 0 || len(f.envelope.purposes) != 0 || len(f.journal.entries) != 0 {
		t.Error("a refused configuration left something behind")
	}

	created, err := f.registry.Invoke(t.Context(), CreateInstanceBackupTargetName, operator(), targetInput("fresh-proof"))
	if err != nil {
		t.Fatalf("configuring: %v", err)
	}
	_, err = f.registry.Invoke(t.Context(), DeleteInstanceBackupTargetName, operator(),
		usecase.Input{"target_id": created.String("id"), "step_up_token": "stale"})
	if code := shared.AsError(err); code == nil || code.DetailCode != "auth.step_up_required" {
		t.Fatalf("with a stale proof: %v", err)
	}
	if len(f.targets.rows) != 1 {
		t.Error("removed without a fresh proof")
	}
}

// A workspace's own credential - its owner's token, without the control plane's scope - reaches
// none of it, reads included.
func TestAWorkspaceTokenReachesNoInstallationTarget(t *testing.T) {
	f := newTargetsFixture(t)
	owner := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: shared.ID("018f2a1b-0000-7000-8000-0000000000c1"),
		AccountID: shared.ID("018f2a1b-0000-7000-8000-0000000000c2"), AccountName: "Owner",
		Scopes: []string{"backup:read", "backup:manage"},
	}
	for name, in := range map[string]usecase.Input{
		ListInstanceBackupTargetsName:  {},
		CreateInstanceBackupTargetName: targetInput("fresh-proof"),
		TestInstanceBackupTargetName:   {"target_id": "018f2a1b-0000-7000-8000-0000000000c3"},
		DeleteInstanceBackupTargetName: {"target_id": "018f2a1b-0000-7000-8000-0000000000c3", "step_up_token": "fresh-proof"},
	} {
		if _, err := f.registry.Invoke(t.Context(), name, owner, in); !errors.Is(err, shared.ErrForbidden) {
			t.Errorf("%s as a workspace's owner: %v", name, err)
		}
	}
	if f.stepUp.consumed != 0 {
		t.Error("a refused caller spent a proof")
	}
}
