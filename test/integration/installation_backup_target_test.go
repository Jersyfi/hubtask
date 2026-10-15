// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The installation's own backup targets (migration 0157), as the application role sees them: a
// session with no tenant writes and reads them, and a session with a tenant reaches none of them -
// per repository method, the cross-tenant rule of SG-3 with the installation as the other side.

func installationTarget(t *testing.T, name string) domain.Target {
	t.Helper()
	target, err := domain.NewTarget(domain.NewTargetInput{
		ID: freshID(t), Name: name, Kind: domain.KindS3,
		Config: domain.TargetConfig{
			"bucket": "installation-exports", "region": "eu-central-1",
			"endpoint": "https://s3.example.org",
		},
		CreatedBy: authorA, Now: created,
	})
	if err != nil {
		t.Fatalf("building the installation's target: %v", err)
	}
	return target
}

func asInstallation(ctx context.Context, t *testing.T, writes bool, fn func(context.Context) error) error {
	t.Helper()
	work := postgres.NewUnitOfWork(appPool(ctx, t))
	if writes {
		return work.Within(ctx, persistence.SystemScope(), fn)
	}
	return work.WithinReadOnly(ctx, persistence.InstallationScope(), fn)
}

func TestTheInstallationsTargetIsItsOwnAndNoWorkspacesToo(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	target := installationTarget(t, freshName(t))
	if err := asInstallation(ctx, t, true, func(ctx context.Context) error {
		return targetRepo().Insert(ctx, target, sealedCredential("the-operators-secret"))
	}); err != nil {
		t.Fatalf("writing the installation's target: %v", err)
	}
	t.Cleanup(func() {
		_ = asInstallation(context.Background(), t, true, func(ctx context.Context) error {
			_, err := targetRepo().Delete(ctx, target.ID)
			return err
		})
	})

	// The installation reads it back, credential included, and lists it.
	if err := asInstallation(ctx, t, false, func(ctx context.Context) error {
		found, err := targetRepo().Find(ctx, target.ID)
		if err != nil {
			return err
		}
		if !found.IsInstanceWide() || found.Config.Get("bucket") != "installation-exports" {
			t.Errorf("read back %+v", found)
		}
		if sealed, err := targetRepo().Credential(ctx, target.ID); err != nil || sealed.IsZero() {
			t.Errorf("the credential: (%v, %v)", sealed.IsZero(), err)
		}
		listed, err := targetRepo().List(ctx)
		if err != nil {
			return err
		}
		for _, each := range listed {
			if !each.IsInstanceWide() {
				t.Errorf("the installation's list holds a workspace's target %s", each.ID)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("reading as the installation: %v", err)
	}

	// A workspace reaches none of it, method by method.
	for name, fn := range map[string]func(context.Context) error{
		"Find": func(ctx context.Context) error {
			_, err := targetRepo().Find(ctx, target.ID)
			return err
		},
		"Credential": func(ctx context.Context) error {
			_, err := targetRepo().Credential(ctx, target.ID)
			return err
		},
		"RecordTest": func(ctx context.Context) error {
			return targetRepo().RecordTest(ctx, target.ID, created, true, "")
		},
	} {
		if err := write(ctx, t, tenantA, fn); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("%s in a workspace reached the installation's target: %v", name, err)
		}
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		removed, err := targetRepo().Delete(ctx, target.ID)
		if removed {
			t.Error("a workspace removed the installation's target")
		}
		return err
	}); err != nil {
		t.Fatalf("deleting from a workspace: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		listed, err := targetRepo().List(ctx)
		for _, each := range listed {
			if each.ID == target.ID {
				t.Error("the installation's target is in a workspace's list")
			}
		}
		return err
	}); err != nil {
		t.Fatalf("listing in a workspace: %v", err)
	}

	// Nor can a workspace write a target that belongs to nobody.
	planted := installationTarget(t, freshName(t))
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return targetRepo().Insert(ctx, planted, sealedCredential("planted"))
	}); err == nil {
		t.Error("a workspace wrote a target of the installation")
	}
}

// The installation, for its part, reaches no workspace's target.
func TestTheInstallationReachesNoWorkspacesTarget(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	target := targetIn(t, tenantA, authorA, freshName(t))
	insertTarget(ctx, t, tenantA, target, sealedCredential("the-bucket-secret"))

	if err := asInstallation(ctx, t, false, func(ctx context.Context) error {
		_, err := targetRepo().Find(ctx, target.ID)
		return err
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("the installation read a workspace's target: %v", err)
	}
	if err := asInstallation(ctx, t, true, func(ctx context.Context) error {
		removed, err := targetRepo().Delete(ctx, target.ID)
		if removed {
			t.Error("the installation removed a workspace's target")
		}
		return err
	}); err != nil {
		t.Fatalf("deleting as the installation: %v", err)
	}
}

// Two installation targets cannot share a name, as two of one workspace cannot.
func TestTwoInstallationTargetsCannotShareAName(t *testing.T) {
	ctx := context.Background()
	name := freshName(t)
	first, second := installationTarget(t, name), installationTarget(t, name)
	for _, target := range []domain.Target{first, second} {
		t.Cleanup(func() {
			_ = asInstallation(context.Background(), t, true, func(ctx context.Context) error {
				_, err := targetRepo().Delete(ctx, target.ID)
				return err
			})
		})
	}
	if err := asInstallation(ctx, t, true, func(ctx context.Context) error {
		return targetRepo().Insert(ctx, first, sealedCredential("one"))
	}); err != nil {
		t.Fatalf("the first: %v", err)
	}
	err := asInstallation(ctx, t, true, func(ctx context.Context) error {
		return targetRepo().Insert(ctx, second, sealedCredential("two"))
	})
	if code := shared.AsError(err); code == nil || code.DetailCode != "backup.target_name_taken" {
		t.Fatalf("the second: %v", err)
	}
}
