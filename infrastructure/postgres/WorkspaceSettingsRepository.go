// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// WorkspaceSettingsRepository is the tenant's own row as the people inside it read and change
// it (F4-01). Neither statement names a tenant: row level security has bound the transaction to
// one, and that is what makes another workspace invisible rather than forbidden.
//
// Named for what it serves rather than for the row, because `WorkspaceRepository` is taken by
// the restore's one-column reader (E-06) and two types with one name would have to live in two
// packages to be told apart.
type WorkspaceSettingsRepository struct{}

func NewWorkspaceSettingsRepository() WorkspaceSettingsRepository {
	return WorkspaceSettingsRepository{}
}

var _ repository.Workspaces = WorkspaceSettingsRepository{}

// settingsDocument is the modelled half of `tenant.settings`, and this file is the only place
// that knows its shape (`TenantPolicy`'s discipline, and `quotasDocument`'s beside it). The
// unmodelled keys never travel through here at all - the write merges rather than replaces.
type settingsDocument struct {
	RequireAdminTotp bool `json:"require_admin_totp"`
	// AuditAnchorTargetID names the backup target the chain's end is anchored to daily (P-13);
	// empty is anchoring switched off, and it is written empty rather than omitted because the
	// write merges keys - an omitted key would leave the old target standing.
	AuditAnchorTargetID string `json:"audit_anchor_target_id"`
	// SignInPolicy is what this workspace decided about signing in (ADR-0068 §2). A *patch*, and
	// stored as one: every field is omitted when the workspace never touched that switch, because a
	// zero written here would be a decision the workspace did not make - and would freeze the
	// instance's default at whatever it happened to be on the day of the save.
	SignInPolicy *policyDocument `json:"sign_in_policy,omitempty"`
}

// policyDocument is the stored patch, flat - the same shape the contract patches, so that the wire
// and the row spell a switch the same way and neither has to be translated into the other.
type policyDocument struct {
	MinLength       *int  `json:"min_length,omitempty"`
	MinLowercase    *int  `json:"min_lowercase,omitempty"`
	MinUppercase    *int  `json:"min_uppercase,omitempty"`
	MinDigits       *int  `json:"min_digits,omitempty"`
	MinSymbols      *int  `json:"min_symbols,omitempty"`
	MinClasses      *int  `json:"min_classes,omitempty"`
	MaxRepeat       *int  `json:"max_repeat,omitempty"`
	CommonPasswords *bool `json:"common_passwords,omitempty"`
	ContextWords    *bool `json:"context_words,omitempty"`
	BreachCheck     *bool `json:"breach_check,omitempty"`
	MaxAgeDays      *int  `json:"max_age_days,omitempty"`
	HistoryCount    *int  `json:"history_count,omitempty"`
	MinAgeHours     *int  `json:"min_age_hours,omitempty"`

	MfaRequiredFor *string   `json:"mfa_required_for,omitempty"`
	Methods        *[]string `json:"methods,omitempty"`

	SessionMaxDays     *int `json:"session_max_days,omitempty"`
	SessionIdleMinutes *int `json:"session_idle_minutes,omitempty"`

	// RotationFrom is the moment "require a new password from everyone" was pressed. A moment
	// rather than a flag: the flag would have to be cleared by something, and a moment is compared.
	RotationFrom *time.Time `json:"rotation_from,omitempty"`

	// The four links this workspace set for its own sign-in footer (SI-12). Beside the switches
	// rather than under a key of their own, because that is how the contract patches them.
	ImprintURL       *string `json:"imprint_url,omitempty"`
	PrivacyURL       *string `json:"privacy_url,omitempty"`
	TermsURL         *string `json:"terms_url,omitempty"`
	AccessibilityURL *string `json:"accessibility_url,omitempty"`
}

// signInPolicyOf reads the stored patch into the domain's.
func signInPolicyOf(stored *policyDocument) (identity.PolicyPatch, identity.LegalLinks) {
	if stored == nil {
		return identity.PolicyPatch{}, identity.LegalLinks{}
	}
	patch := identity.PolicyPatch{
		MinLength: stored.MinLength, MinLowercase: stored.MinLowercase,
		MinUppercase: stored.MinUppercase, MinDigits: stored.MinDigits,
		MinSymbols: stored.MinSymbols, MinClasses: stored.MinClasses,
		MaxRepeat: stored.MaxRepeat, CommonPasswords: stored.CommonPasswords,
		ContextWords: stored.ContextWords, BreachCheck: stored.BreachCheck,
		MaxAgeDays: stored.MaxAgeDays, HistoryCount: stored.HistoryCount,
		MinAgeHours: stored.MinAgeHours, Methods: stored.Methods,
		SessionMaxDays: stored.SessionMaxDays, SessionIdleMinutes: stored.SessionIdleMinutes,
		RotationFrom: stored.RotationFrom,
	}
	if stored.MfaRequiredFor != nil {
		requirement := identity.MfaRequirement(*stored.MfaRequiredFor)
		patch.MfaRequiredFor = &requirement
	}
	links := identity.LegalLinks{}
	for name, value := range map[identity.LegalLink]*string{
		identity.LinkImprint: stored.ImprintURL, identity.LinkPrivacy: stored.PrivacyURL,
		identity.LinkTerms: stored.TermsURL, identity.LinkAccessibility: stored.AccessibilityURL,
	} {
		if value != nil {
			links = links.With(name, *value)
		}
	}
	return patch, links
}

// signInDocumentOf writes the domain's patch back. A switch the workspace never decided is omitted, which
// is what keeps the row a patch rather than a snapshot.
func signInDocumentOf(patch identity.PolicyPatch, links identity.LegalLinks) *policyDocument {
	if patch.IsEmpty() && links.IsEmpty() {
		return nil
	}
	stored := &policyDocument{
		MinLength: patch.MinLength, MinLowercase: patch.MinLowercase,
		MinUppercase: patch.MinUppercase, MinDigits: patch.MinDigits,
		MinSymbols: patch.MinSymbols, MinClasses: patch.MinClasses,
		MaxRepeat: patch.MaxRepeat, CommonPasswords: patch.CommonPasswords,
		ContextWords: patch.ContextWords, BreachCheck: patch.BreachCheck,
		MaxAgeDays: patch.MaxAgeDays, HistoryCount: patch.HistoryCount,
		MinAgeHours: patch.MinAgeHours, Methods: patch.Methods,
		SessionMaxDays: patch.SessionMaxDays, SessionIdleMinutes: patch.SessionIdleMinutes,
		RotationFrom: patch.RotationFrom,
	}
	if patch.MfaRequiredFor != nil {
		requirement := string(*patch.MfaRequiredFor)
		stored.MfaRequiredFor = &requirement
	}
	for name, field := range map[identity.LegalLink]**string{
		identity.LinkImprint: &stored.ImprintURL, identity.LinkPrivacy: &stored.PrivacyURL,
		identity.LinkTerms: &stored.TermsURL, identity.LinkAccessibility: &stored.AccessibilityURL,
	} {
		if value := links.Of(name); value != "" {
			held := value
			*field = &held
		}
	}
	return stored
}

// Find answers the workspace the transaction is bound to.
func (WorkspaceSettingsRepository) Find(ctx context.Context) (identity.Workspace, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return identity.Workspace{}, err
	}

	row, err := queries.FindWorkspace(ctx)
	if err != nil {
		if IsNoRows(err) {
			return identity.Workspace{}, shared.ErrNotFound.WithDetail("admin.tenant_not_found")
		}
		return identity.Workspace{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the workspace: %w", err))
	}

	id, err := idFrom(row.ID)
	if err != nil {
		return identity.Workspace{}, err
	}

	var settings settingsDocument
	if len(row.Settings) > 0 {
		if err := json.Unmarshal(row.Settings, &settings); err != nil {
			// The same fail-closed answer `RequireAdminTotp` gives: a settings document this
			// build cannot read must not be reported as one with enforcement switched off.
			return identity.Workspace{}, shared.ErrInternal.
				WithDetail("postgres.query_failed").
				WithCause(fmt.Errorf("parsing the workspace settings: %w", err))
		}
	}

	patch, links := signInPolicyOf(settings.SignInPolicy)

	return identity.Workspace{
		Tenant: identity.Tenant{
			ID: id, Slug: row.Slug, DisplayName: row.DisplayName,
			Status:        identity.TenantStatus(row.Status),
			DefaultLocale: row.DefaultLocale, DefaultTimeZone: row.DefaultTimeZone,
			CreatedAt: timeFrom(row.CreatedAt),
		},
		Settings: identity.WorkspaceSettings{
			RequireAdminTotp:    settings.RequireAdminTotp,
			AuditAnchorTargetID: shared.ID(settings.AuditAnchorTargetID),
			SignIn:              patch,
			Legal:               links,
		},
		UpdatedAt: timeFrom(row.UpdatedAt),
		Version:   int(row.Version),
	}, nil
}

// Update writes the three columns and merges the modelled settings keys, guarded on the version.
func (WorkspaceSettingsRepository) Update(
	ctx context.Context, changed identity.Workspace, expectedVersion int, now time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}

	payload, err := json.Marshal(settingsDocument{
		RequireAdminTotp:    changed.Settings.RequireAdminTotp,
		AuditAnchorTargetID: changed.Settings.AuditAnchorTargetID.String(),
		SignInPolicy:        signInDocumentOf(changed.Settings.SignIn, changed.Settings.Legal),
	})
	if err != nil {
		return false, shared.Internalf("postgres: encoding the workspace settings: %w", err)
	}

	rows, err := queries.UpdateWorkspace(ctx, sqlc.UpdateWorkspaceParams{
		DisplayName:     changed.DisplayName,
		DefaultLocale:   changed.DefaultLocale,
		DefaultTimeZone: changed.DefaultTimeZone,
		Settings:        payload,
		Now:             pgtype.Timestamptz{Time: now, Valid: true},
		//nolint:gosec // G115: a row version, bounded far below either type's range
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the workspace: %w", err))
	}
	return rows > 0, nil
}
