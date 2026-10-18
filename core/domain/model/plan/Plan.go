// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package plan holds a plan: a provider's group of workspaces with its own limits, locked values
// and the features it leaves out, between the installation and the workspace
// (multi-tenancy.md §4.2, ADR-0086).
//
// A plan is a level, not a template: it holds only what it decides, and the rest falls through to
// the installation. What it may decide is the installation's to bound - an installation lock binds
// the plan as it binds a workspace, and a plan only tightens what the installation left open. That
// check is here, once, for every door that writes a plan.
package plan

import (
	"regexp"
	"slices"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/entitlement"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Feature is one of the closed set a plan may leave out (multi-tenancy.md §4.2), held in its own
// package so that a workspace can carry the set its plan leaves out (UC-INS-13 check 5).
type Feature = entitlement.Feature

const (
	FeatureAutomationRules    = entitlement.AutomationRules
	FeatureWebhooks           = entitlement.Webhooks
	FeatureOwnSignInProviders = entitlement.OwnSignInProviders
)

// Features is the closed set, in the order a screen draws it.
func Features() []Feature { return entitlement.Features() }

// Values is what a plan decides: the four areas of the installation's level a plan may reach, each
// with its locks, and the features it leaves out. No localisation: a plan sets limits, locks and
// features, and a language is never locked anywhere (multi-tenancy.md §4.2).
type Values struct {
	Policy  identity.PolicyLayer
	Legal   identity.LegalLayer
	Quotas  identity.QuotaDefaults
	Ai      integration.AiLayer
	LeftOut []Feature
}

// Above is what the installation decided, as a plan is checked against it.
type Above struct {
	Policy identity.PolicyLayer
	Legal  identity.LegalLayer
	Quotas identity.QuotaDefaults
	Ai     integration.AiLayer
}

// Plan is one plan as the control plane holds it.
type Plan struct {
	ID          shared.ID
	Name        string
	DisplayName string
	Version     int
	Values      Values
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Includes reports whether a workspace on the plan has the feature.
func (p Plan) Includes(feature Feature) bool { return !slices.Contains(p.Values.LeftOut, feature) }

// namePattern is the short name: what a platform maps its products to, so it is a label a URL and
// a configuration file carry unchanged.
var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,39}$`)

// MaxDisplayNameLength bounds the operator's own name for a plan.
const MaxDisplayNameLength = 200

// ValidName refuses a short name that is not a label.
func ValidName(name string) error {
	if namePattern.MatchString(name) {
		return nil
	}
	return shared.ErrValidation.WithDetail("admin.plan_name_invalid").
		WithFields(shared.FieldError{Path: "/name", Code: "admin.plan_name_invalid"})
}

// ValidDisplayName refuses an empty or overlong display name.
func ValidDisplayName(name string) error {
	if length := len([]rune(name)); length >= 1 && length <= MaxDisplayNameLength {
		return nil
	}
	return shared.ErrValidation.WithDetail("admin.plan_display_name_invalid").
		WithFields(shared.FieldError{Path: "/display_name", Code: "admin.plan_display_name_invalid"})
}

// FitsUnder checks a plan's values against what the installation decided (multi-tenancy.md
// §4.2): a key the installation locks is not the plan's to set, and a value is no looser
// than the one the installation's level leaves in force. A limit has no direction a plan must keep
// - where the installation leaves it open, a plan may raise it as a workspace's exception may
// (`MayRaise`) - so only its lock is checked.
func (v Values) FitsUnder(above Above) error {
	if err := v.policyFits(above.Policy); err != nil {
		return err
	}
	if err := v.legalFits(above.Legal); err != nil {
		return err
	}
	for _, name := range quotaNames(v.Quotas) {
		if above.Quotas.Locked(name) {
			return underLock("quotas", name)
		}
	}
	if err := v.aiFits(above.Ai); err != nil {
		return err
	}
	for _, feature := range v.LeftOut {
		if !feature.Valid() {
			return shared.ErrValidation.WithDetail("admin.plan_feature_unknown").
				WithParams(map[string]string{"feature": string(feature)}).
				WithFields(shared.FieldError{Path: "/left_out", Code: "admin.plan_feature_unknown"})
		}
	}
	return nil
}

func (v Values) policyFits(above identity.PolicyLayer) error {
	inForce := identity.Effective(above, identity.PolicyLayer{}, identity.PolicyLayer{}).Policy
	for _, name := range v.Policy.Patch.Decided() {
		if above.Locks[name] {
			return underLock("sign_in", string(name))
		}
		if identity.Tighten(name, inForce, v.Policy.Patch) != nil {
			return looser("sign_in", string(name))
		}
	}
	return nil
}

func (v Values) legalFits(above identity.LegalLayer) error {
	if v.Legal.Locks[identity.LinkAccount] {
		// No level below a plan sets the account link, so a plan's lock on it would bind nobody.
		return shared.ErrValidation.WithDetail("admin.plan_account_link_not_lockable").
			WithFields(shared.FieldError{Path: "/legal/" + string(identity.LinkAccount), Code: "admin.plan_account_link_not_lockable"})
	}
	for _, link := range identity.LevelLegalSettingNames() {
		if v.Legal.Links.Of(link) == "" {
			continue
		}
		// The version is locked exactly when its link is (identity.md §18).
		lockedBy := link
		if link == identity.LinkTermsVersion {
			lockedBy = identity.LinkTerms
		}
		if above.Locks[lockedBy] {
			return underLock("legal", string(link))
		}
	}
	if v.Legal.Links.Of(identity.LinkTermsVersion) != "" && v.Legal.Links.Of(identity.LinkTerms) == "" {
		return identity.TermsVersionWithoutLink()
	}
	return nil
}

func (v Values) aiFits(above integration.AiLayer) error {
	inForce := integration.EffectiveAi(above, integration.AiLayer{}, integration.AiLayer{}, true)
	if v.Ai.Sources != nil {
		if above.Locks[integration.AiSwitchSources] {
			return underLock("ai", string(integration.AiSwitchSources))
		}
		if !v.Ai.Sources.Within(inForce.Above) {
			return looser("ai", string(integration.AiSwitchSources))
		}
	}
	if v.Ai.MinJurisdiction != nil {
		if above.Locks[integration.AiSwitchMinJurisdiction] {
			return underLock("ai", string(integration.AiSwitchMinJurisdiction))
		}
		order := integration.AiMinJurisdictions()
		if slices.Index(order, *v.Ai.MinJurisdiction) < slices.Index(order, inForce.MinJurisdiction) {
			return looser("ai", string(integration.AiSwitchMinJurisdiction))
		}
	}
	if v.Ai.PersonOptOut != nil && above.Locks[integration.AiSwitchPersonOptOut] {
		return underLock("ai", string(integration.AiSwitchPersonOptOut))
	}
	return nil
}

// quotaNames is every quota a plan sets or locks, in a stable order so that a refusal names the same
// one each time: a lock is the plan's write as much as a value is.
func quotaNames(quotas identity.QuotaDefaults) []string {
	names := make([]string, 0, len(quotas.Limits)+len(quotas.Locks))
	for name := range quotas.Limits {
		names = append(names, name)
	}
	for name, locked := range quotas.Locks {
		if _, set := quotas.Limits[name]; locked && !set {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

func underLock(area, name string) error {
	return refused("admin.plan_under_installation_lock", area, name)
}

func looser(area, name string) error {
	return refused("admin.plan_looser_than_installation", area, name)
}

func refused(code, area, name string) error {
	return shared.ErrValidation.WithDetail(code).
		WithParams(map[string]string{"setting": area + "." + name}).
		WithFields(shared.FieldError{Path: "/" + area + "/" + name, Code: code})
}
