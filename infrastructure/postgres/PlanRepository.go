// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/admin"
	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/entitlement"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/plan"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// PlanRepository is the installation's plans (multi-tenancy.md §4.2).
//
// A plan's rows are the installation level's own key shape, read and written through the level's
// one mapping (`applyRow`, `rowsOf`), so a key means the same at both levels and a key added to one
// is added to both. What only a plan has - the features it leaves out - is `entitlement.<feature>`.
type PlanRepository struct{}

// NewPlanRepository answers the repository.
func NewPlanRepository() PlanRepository { return PlanRepository{} }

var (
	_ repository.Plans        = PlanRepository{}
	_ repository.Entitlements = PlanRepository{}
)

const entitlementArea = "entitlement"

// List answers every plan not removed, oldest first, with what each decides.
func (PlanRepository) List(ctx context.Context) ([]plan.Plan, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListPlans(ctx)
	if err != nil {
		return nil, queryFailed("listing the plans", err)
	}
	plans := make([]plan.Plan, 0, len(rows))
	ids := make([]pgtype.UUID, 0, len(rows))
	for _, row := range rows {
		read, err := planOf(row.ID, row.Name, row.DisplayName, row.Version, row.CreatedAt, row.UpdatedAt)
		if err != nil {
			return nil, err
		}
		plans = append(plans, read)
		ids = append(ids, row.ID)
	}
	if err := withSettings(ctx, queries, plans, ids); err != nil {
		return nil, err
	}
	return plans, nil
}

// Find answers one plan not removed by its short name.
func (PlanRepository) Find(ctx context.Context, name string) (plan.Plan, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return plan.Plan{}, err
	}
	row, err := queries.FindPlanByName(ctx, name)
	if errors.Is(err, pgx.ErrNoRows) {
		return plan.Plan{}, shared.ErrNotFound.WithDetail("admin.plan_not_found")
	}
	if err != nil {
		return plan.Plan{}, queryFailed("reading a plan", err)
	}
	found, err := planOf(row.ID, row.Name, row.DisplayName, row.Version, row.CreatedAt, row.UpdatedAt)
	if err != nil {
		return plan.Plan{}, err
	}
	plans := []plan.Plan{found}
	if err := withSettings(ctx, queries, plans, []pgtype.UUID{row.ID}); err != nil {
		return plan.Plan{}, err
	}
	return plans[0], nil
}

// Hold answers a plan's identifier and holds it against a removal until the transaction ends.
func (PlanRepository) Hold(ctx context.Context, name string) (shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", err
	}
	id, err := queries.HoldPlanByName(ctx, name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", shared.ErrNotFound.WithDetail("admin.plan_not_found")
	}
	if err != nil {
		return "", queryFailed("holding a plan", err)
	}
	return idFrom(id)
}

// Ever reports whether any plan was ever created, removed ones included.
func (PlanRepository) Ever(ctx context.Context) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	ever, err := queries.AnyPlanEver(ctx)
	if err != nil {
		return false, queryFailed("asking whether a plan exists", err)
	}
	return ever, nil
}

// Workspaces answers how many workspaces are on each plan.
func (PlanRepository) Workspaces(ctx context.Context) (map[string]int64, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.PlanWorkspaceCounts(ctx)
	if err != nil {
		return nil, queryFailed("counting the workspaces on each plan", err)
	}
	counts := make(map[string]int64, len(rows))
	for _, row := range rows {
		counts[row.PlanName] = row.Workspaces
	}
	return counts, nil
}

// Insert writes a new plan with its values.
func (PlanRepository) Insert(ctx context.Context, created plan.Plan, by shared.ID) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(created.ID)
	if err != nil {
		return err
	}
	actor, err := actorOf(by)
	if err != nil {
		return err
	}
	inserted, err := queries.InsertPlan(ctx, sqlc.InsertPlanParams{
		ID: id, Name: created.Name, DisplayName: created.DisplayName,
		Now: pgtype.Timestamptz{Time: created.CreatedAt.UTC(), Valid: true}, UpdatedBy: actor,
	})
	if err != nil {
		return queryFailed("writing a plan", err)
	}
	if inserted == 0 {
		// The name stands in a row, removed or not: a short name is never reused.
		return shared.ErrConflict.WithDetail("admin.plan_name_taken")
	}
	return writeSettings(ctx, queries, id, created.Values)
}

// Replace writes a plan's display name and values whole.
func (PlanRepository) Replace(
	ctx context.Context, changed plan.Plan, expected *int, by shared.ID, now time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(changed.ID)
	if err != nil {
		return false, err
	}
	actor, err := actorOf(by)
	if err != nil {
		return false, err
	}
	var version *int32
	if expected != nil {
		guarded := int32(*expected) //nolint:gosec // a version is a small counter
		version = &guarded
	}
	updated, err := queries.UpdatePlan(ctx, sqlc.UpdatePlanParams{
		DisplayName: changed.DisplayName, Now: pgtype.Timestamptz{Time: now.UTC(), Valid: true},
		UpdatedBy: actor, ID: id, Version: version,
	})
	if err != nil {
		return false, queryFailed("changing a plan", err)
	}
	if updated == 0 {
		return false, nil
	}
	if err := queries.ClearPlanSettings(ctx, id); err != nil {
		return false, queryFailed("clearing a plan's values", err)
	}
	return true, writeSettings(ctx, queries, id, changed.Values)
}

// Remove ends a plan unless a workspace is on it.
func (PlanRepository) Remove(
	ctx context.Context, planID shared.ID, by shared.ID, now time.Time,
) (repository.RemoveOutcome, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", err
	}
	id, err := uuidOf(planID)
	if err != nil {
		return "", err
	}
	actor, err := actorOf(by)
	if err != nil {
		return "", err
	}
	outcome, err := queries.RemovePlan(ctx, sqlc.RemovePlanParams{
		ID: id, Now: pgtype.Timestamptz{Time: now.UTC(), Valid: true}, Actor: actor,
	})
	if err != nil {
		return "", queryFailed("removing a plan", err)
	}
	return repository.RemoveOutcome(outcome), nil
}

func planOf(
	id pgtype.UUID, name, displayName string, version int32, created, updated pgtype.Timestamptz,
) (plan.Plan, error) {
	planID, err := idFrom(id)
	if err != nil {
		return plan.Plan{}, err
	}
	return plan.Plan{
		ID: planID, Name: name, DisplayName: displayName, Version: int(version),
		CreatedAt: timeFrom(created), UpdatedAt: timeFrom(updated),
	}, nil
}

// withSettings reads what the plans decide and folds it into each, through the level's mapping.
func withSettings(ctx context.Context, queries *sqlc.Queries, plans []plan.Plan, ids []pgtype.UUID) error {
	if len(plans) == 0 {
		return nil
	}
	rows, err := queries.PlanSettings(ctx, ids)
	if err != nil {
		return queryFailed("reading the plans' values", err)
	}
	levels := make(map[shared.ID]*identityrepo.InstanceLevel, len(plans))
	leftOut := make(map[shared.ID][]plan.Feature, len(plans))
	for _, row := range rows {
		planID, err := idFrom(row.PlanID)
		if err != nil {
			return err
		}
		if feature, isFeature, err := leftOutBy(row.Key, row.Value); isFeature {
			if err != nil {
				return err
			}
			if feature != "" {
				leftOut[planID] = append(leftOut[planID], feature)
			}
			continue
		}
		level, held := levels[planID]
		if !held {
			level = emptyLevel()
			levels[planID] = level
		}
		if err := applyRow(level, row.Key, row.Value, row.Locked); err != nil {
			return err
		}
	}
	for i := range plans {
		held := &plans[i]
		level, decided := levels[held.ID]
		if !decided {
			level = emptyLevel()
		}
		held.Values = plan.Values{
			Policy: level.Policy, Legal: level.Legal, Quotas: level.Quotas, Ai: level.Ai,
			Modules: level.Modules, LeftOut: orderedFeatures(leftOut[held.ID]),
		}
	}
	return nil
}

// leftOutBy reads one plan row as a feature: whether the row is one, and the feature where it
// leaves it out. A feature's row holds whether it is included.
func leftOutBy(key string, value []byte) (plan.Feature, bool, error) {
	name, isFeature := strings.CutPrefix(key, entitlementArea+".")
	if !isFeature {
		return "", false, nil
	}
	var included bool
	if err := json.Unmarshal(value, &included); err != nil {
		return "", true, malformedSetting(key, err)
	}
	if included {
		return "", true, nil
	}
	return plan.Feature(strings.ToUpper(name)), true, nil
}

// orderedFeatures puts the features in the domain's order, so that two reads of one plan answer
// the same list.
func orderedFeatures(features []plan.Feature) []plan.Feature { return entitlement.Ordered(features) }

func emptyLevel() *identityrepo.InstanceLevel {
	return &identityrepo.InstanceLevel{
		Policy:  identity.PolicyLayer{Locks: map[identity.PolicySwitch]bool{}},
		Legal:   identity.LegalLayer{Locks: map[identity.LegalLink]bool{}},
		Quotas:  identity.QuotaDefaults{Limits: map[string]int64{}, Locks: map[string]bool{}},
		Ai:      integration.AiLayer{Locks: map[integration.AiSwitch]bool{}},
		Modules: module.Decisions{},
	}
}

// writeSettings writes what a plan decides, in the level's key shape, and a row per feature it
// leaves out.
func writeSettings(ctx context.Context, queries *sqlc.Queries, id pgtype.UUID, values plan.Values) error {
	rows, err := rowsOf(identityrepo.InstanceLevel{
		Policy: values.Policy, Legal: values.Legal, Quotas: values.Quotas, Ai: values.Ai,
		Modules: values.Modules,
	})
	if err != nil {
		return err
	}
	for _, feature := range values.LeftOut {
		rows = append(rows, settingRow{
			key: entitlementArea + "." + strings.ToLower(string(feature)), value: []byte("false"), lock: "OPEN",
		})
	}
	for _, row := range rows {
		if err := queries.PutPlanSetting(ctx, sqlc.PutPlanSettingParams{
			PlanID: id, Key: row.key, Value: row.value, Locked: row.lock != "OPEN",
		}); err != nil {
			return queryFailed(fmt.Sprintf("writing the plan setting %q", row.key), err)
		}
	}
	return nil
}

func actorOf(by shared.ID) (pgtype.UUID, error) {
	if by.IsZero() {
		return pgtype.UUID{}, nil
	}
	return uuidOf(by)
}

// LeftOut answers what the transaction's own workspace's plan leaves out, empty where it is on
// none (admin.Entitlements).
func (PlanRepository) LeftOut(ctx context.Context) (entitlement.LeftOut, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.LeftOutOfWorkspace(ctx)
	if err != nil {
		return nil, queryFailed("reading what the workspace's plan leaves out", err)
	}
	held := make([]plan.Feature, 0, len(rows))
	for _, row := range rows {
		feature, _, err := leftOutBy(row.Key, row.Value)
		if err != nil {
			return nil, err
		}
		if feature != "" {
			held = append(held, feature)
		}
	}
	return entitlement.Ordered(held), nil
}

// OfWorkspace answers the transaction's own workspace's plan, and false where it is on none.
func (PlanRepository) OfWorkspace(ctx context.Context) (plan.Plan, bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return plan.Plan{}, false, err
	}
	row, err := queries.PlanOfWorkspace(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return plan.Plan{}, false, nil
	}
	if err != nil {
		return plan.Plan{}, false, queryFailed("reading the workspace's plan", err)
	}
	held, err := planOf(row.ID, row.Name, row.DisplayName, row.Version, row.CreatedAt, row.UpdatedAt)
	if err != nil {
		return plan.Plan{}, false, err
	}
	plans := []plan.Plan{held}
	if err := withSettings(ctx, queries, plans, []pgtype.UUID{row.ID}); err != nil {
		return plan.Plan{}, false, err
	}
	return plans[0], true, nil
}

// PlanAi answers the transaction's workspace's plan's AI layer, the zero layer on no plan.
func (PlanRepository) PlanAi(ctx context.Context) (integration.AiLayer, error) {
	held, on, err := PlanRepository{}.OfWorkspace(ctx)
	if err != nil || !on {
		return integration.AiLayer{}, err
	}
	return held.Values.Ai, nil
}
