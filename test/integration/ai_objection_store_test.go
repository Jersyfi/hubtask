// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// Who keeps their content out of AI, as the database answers it: the newest record per person
// decides, and nothing of it is visible from another workspace (UC-PRV-05 checks 5 and 12).

type objectionPeople struct {
	tenant, other         shared.ID
	keeper, giver, silent shared.ID
	stranger              shared.ID
}

func seedObjectionPeople(ctx context.Context, t *testing.T) objectionPeople {
	t.Helper()
	admin := adminPool(ctx, t)
	people := objectionPeople{
		tenant: freshID(t), other: freshID(t),
		keeper: freshID(t), giver: freshID(t), silent: freshID(t), stranger: freshID(t),
	}
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenant (id, slug, display_name) VALUES ($1, $2, 'Objection'), ($3, $4, 'Next door')`,
			[]any{people.tenant.String(), slugOf(people.tenant), people.other.String(), slugOf(people.other)}},
		{`INSERT INTO account (id, tenant_id, email, display_name, sign_in_name) VALUES
		    ($1, $4, 'keeper@example.org', 'Kim Keeper', 'kimk'),
		    ($2, $4, 'giver@example.org', 'Gil Giver', NULL),
		    ($3, $4, 'silent@example.org', 'Sam Silent', NULL)`,
			[]any{people.keeper.String(), people.giver.String(), people.silent.String(), people.tenant.String()}},
		{`INSERT INTO account (id, tenant_id, email, display_name) VALUES ($1, $2, 'stranger@example.org', 'Stan Stranger')`,
			[]any{people.stranger.String(), people.other.String()}},
	} {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding: %v\n%s", err, statement.sql)
		}
	}
	return people
}

func TestTheNewestRecordDecidesWhoKeepsTheirContentOut(t *testing.T) {
	ctx := context.Background()
	people := seedObjectionPeople(ctx, t)
	repo := privacyRepo()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)

	record := func(tenant, account shared.ID, granted bool, when time.Time) {
		inTenant(t, uow, tenant, func(ctx context.Context) error {
			consent := domain.Consent{
				ID: freshID(t), AccountID: account, Purpose: domain.PurposeAi,
				Granted: granted, GrantedAt: when, Source: domain.SourceUser,
			}
			if !granted {
				consent.RevokedAt = when
			}
			return repo.Record(ctx, consent)
		})
	}
	// The keeper withdrew. The giver withdrew and then gave it again. The silent one never said
	// anything. The stranger withdrew next door.
	record(people.tenant, people.keeper, false, at)
	record(people.tenant, people.giver, false, at)
	record(people.tenant, people.giver, true, at.Add(time.Hour))
	record(people.other, people.stranger, false, at)

	inTenant(t, uow, people.tenant, func(ctx context.Context) error {
		standing, err := repo.Standing(ctx, domain.PurposeAi)
		if err != nil {
			t.Fatalf("standing: %v", err)
		}
		if len(standing) != 1 || standing[0].AccountID != people.keeper {
			t.Fatalf("standing %+v, want the keeper alone", standing)
		}
		kim := standing[0]
		if kim.DisplayName != "Kim Keeper" || kim.Email != "keeper@example.org" || kim.SignInName != "kimk" ||
			!kim.Since.Equal(at) || kim.Source != domain.SourceUser {
			t.Errorf("the keeper read back as %+v", kim)
		}
		other, err := repo.Standing(ctx, "metering")
		if err != nil || len(other) != 0 {
			t.Errorf("another purpose answered %+v (%v)", other, err)
		}
		return nil
	})

	// Next door sees its own stranger and nobody of ours.
	inTenant(t, uow, people.other, func(ctx context.Context) error {
		standing, err := repo.Standing(ctx, domain.PurposeAi)
		if err != nil {
			t.Fatalf("standing next door: %v", err)
		}
		if len(standing) != 1 || standing[0].AccountID != people.stranger {
			t.Errorf("next door read %+v", standing)
		}
		return nil
	})
}

func TestANoticeIsAcknowledgedForwardsOnlyAndStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	people := seedObjectionPeople(ctx, t)
	repo := privacyRepo()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	first := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)

	inTenant(t, uow, people.tenant, func(ctx context.Context) error {
		if at, err := repo.NoticedAt(ctx, people.keeper); err != nil || !at.IsZero() {
			t.Errorf("never noticed answered %v (%v)", at, err)
		}
		if err := repo.Notice(ctx, people.keeper, first.Add(time.Hour)); err != nil {
			t.Fatalf("noticing: %v", err)
		}
		// A retried, older acknowledgement does not move it back.
		if err := repo.Notice(ctx, people.keeper, first); err != nil {
			t.Fatalf("noticing again: %v", err)
		}
		if at, err := repo.NoticedAt(ctx, people.keeper); err != nil || !at.Equal(first.Add(time.Hour)) {
			t.Errorf("noticed at %v (%v), want the later moment", at, err)
		}
		return nil
	})

	inTenantRolledBack(t, uow, people.other, func(ctx context.Context) {
		if at, err := repo.NoticedAt(ctx, people.keeper); err != nil || !at.IsZero() {
			t.Errorf("next door read our notice: %v (%v)", at, err)
		}
		// Next door cannot write one for our person: the account is not theirs.
		if err := repo.Notice(ctx, people.keeper, first); err == nil {
			t.Error("next door recorded a notice for an account of ours")
		}
	})
}

func TestTheWorkspacesPositionRoundTripsThroughItsSettings(t *testing.T) {
	ctx := context.Background()
	people := seedObjectionPeople(ctx, t)
	workspaces := postgres.NewWorkspaceSettingsRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	since := time.Date(2026, 10, 12, 9, 30, 0, 0, time.UTC)

	inTenant(t, uow, people.tenant, func(ctx context.Context) error {
		found, err := workspaces.Find(ctx)
		if err != nil {
			t.Fatalf("finding: %v", err)
		}
		found.Settings.AiPersonOptOut = "NOT_OFFERED"
		found.Settings.AiOptOutBasis = "WORKS_AGREEMENT"
		found.Settings.AiOptOutContact = "The works council"
		found.Settings.AiOptOutSince = since
		if written, err := workspaces.Update(ctx, found, found.Version, since); err != nil || !written {
			t.Fatalf("writing: %v (%v)", written, err)
		}
		again, err := workspaces.Find(ctx)
		if err != nil {
			t.Fatalf("reading back: %v", err)
		}
		got := again.Settings
		if got.AiPersonOptOut != "NOT_OFFERED" || got.AiOptOutBasis != "WORKS_AGREEMENT" ||
			got.AiOptOutContact != "The works council" || !got.AiOptOutSince.Equal(since) {
			t.Errorf("read back %+v", got)
		}
		// Back to OFFERED clears the basis rather than leaving it in the merge.
		again.Settings.AiPersonOptOut, again.Settings.AiOptOutBasis = "OFFERED", ""
		again.Settings.AiOptOutContact, again.Settings.AiOptOutSince = "", time.Time{}
		if written, err := workspaces.Update(ctx, again, again.Version, since.Add(time.Minute)); err != nil || !written {
			t.Fatalf("writing back: %v (%v)", written, err)
		}
		cleared, err := workspaces.Find(ctx)
		if err != nil {
			t.Fatalf("reading the cleared position: %v", err)
		}
		if cleared.Settings.AiOptOutBasis != "" || cleared.Settings.AiOptOutContact != "" || !cleared.Settings.AiOptOutSince.IsZero() {
			t.Errorf("the basis survived going back: %+v", cleared.Settings)
		}
		return nil
	})
}
