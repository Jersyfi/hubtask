// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The workspace's half of the sign-in rule (ADR-0068 §2).
//
// A workspace only ever tightens, and a switch the level above locked is not its to touch. Both are
// decided in the domain; what this file does is bring the patch a `PATCH /tenant` carried to it, and
// project the answer back in the shape a screen with eighteen rows needs - each row saying what is
// in force here, what the level above set, and whether a lock is on it.
//
// **A locked switch is shown, not hidden.** A setting that simply is not there is a setting somebody
// opens a support ticket about; a setting shown with its value, its reason and a control that is
// switched off is one they understand.

// WorkspacePolicyChange is the sign-in half of a workspace patch.
type WorkspacePolicyChange struct {
	// Policy is the switches the caller sent. A switch it did not send is left standing.
	Policy domain.PolicyPatch
	// Legal is the links the caller sent, by name. An entry set to the empty string clears that
	// link, which is a decision a workspace is allowed to make - unlike a switch, a link has an
	// honest "none".
	Legal map[domain.LegalLink]string
	// RotateNow is "require a new password from everyone", which the contract spells as the literal
	// `now` rather than as a moment: a client that could name one could name a moment in the past
	// and un-require a rotation, or one in the future and arm a trap.
	RotateNow bool
}

// IsEmpty reports whether this patch says anything about signing in.
func (c WorkspacePolicyChange) IsEmpty() bool {
	return c.Policy.IsEmpty() && len(c.Legal) == 0 && !c.RotateNow
}

// translateAdminFlag turns a write of the old boolean into the rule's own switch (UC-ID-12 check 2).
//
// Against the rule in force, because "on" means "administrators or stricter": a workspace whose
// rule reaches everyone already has the flag on, and switching it on again must not loosen the rule
// to administrators. Where the same body also names `mfa_required_for`, that is the newer statement
// of the same thing and is kept - unless the two contradict each other, which is refused against the
// old field rather than guessed at.
func (w WorkspaceWriter) translateAdminFlag(
	ctx context.Context, tenantID shared.ID, flag bool, change WorkspacePolicyChange,
) (WorkspacePolicyChange, error) {
	if sent := change.Policy.MfaRequiredFor; sent != nil {
		if sent.CoversAdmins() != flag {
			return WorkspacePolicyChange{}, shared.ErrValidation.
				WithDetail("workspace.admin_flag_contradicts_rule").
				WithFields(shared.FieldError{
					Path: "/require_admin_totp", Code: "workspace.admin_flag_contradicts_rule",
				})
		}
		return change, nil
	}
	if w.Resolver.Instance == nil {
		// No rule to write into, applyPolicy's reasoning: refusing is more honest than storing a
		// flag nothing reads.
		return WorkspacePolicyChange{}, shared.ErrConflict.WithDetail("auth.policy_unavailable")
	}
	resolved, err := w.Resolver.Resolve(ctx, tenantID)
	if err != nil {
		return WorkspacePolicyChange{}, err
	}
	if wanted, moves := domain.RequirementForAdminFlag(
		flag, resolved.Effective.Policy.MfaRequiredFor); moves {
		change.Policy.MfaRequiredFor = &wanted
	}
	return change, nil
}

// adminFlagOf answers the old boolean from the rule in force, which is the only place it is read
// from (UC-ID-12 check 2). The stored boolean could disagree with the rule - the SI review found it
// did, in both directions - and the rule is what signing in has always obeyed. Without a level
// above to resolve against, the workspace's own layer answers, the old boolean folded into it.
func adminFlagOf(workspace domain.Workspace, resolved *ResolvedPolicy) bool {
	if resolved != nil {
		return resolved.Effective.Policy.MfaRequiredFor.CoversAdmins()
	}
	own := workspace.Settings.SignInLayer().Patch.MfaRequiredFor
	return own != nil && own.CoversAdmins()
}

// applyPolicy folds the sign-in half of a patch into the workspace, refusing what the level above
// forbids, and answers the fields that moved for the trail.
func (w WorkspaceWriter) applyPolicy(
	ctx context.Context, tenantID shared.ID, stored domain.Workspace,
	change WorkspacePolicyChange, now time.Time,
) (domain.Workspace, []domain.FieldChange, error) {
	if change.IsEmpty() {
		return stored, nil, nil
	}
	if w.Resolver.Instance == nil {
		// An installation wired before the instance layer. The resolution would answer the
		// product's default for everything, which is right, but refusing here is more honest than
		// writing a policy nothing will ever read.
		return domain.Workspace{}, nil, shared.ErrConflict.WithDetail("auth.policy_unavailable")
	}

	resolved, err := w.Resolver.Resolve(ctx, tenantID)
	if err != nil {
		return domain.Workspace{}, nil, err
	}

	patch := change.Policy
	if change.RotateNow {
		// The moment is the server's. Every password set before it meets the change step at the
		// next sign-in, and every session opened before it is refused on its next request - which
		// is the whole of the enforcement, with no job and no write into anybody's row.
		moment := now.UTC()
		patch.RotationFrom = &moment
	}

	if _, moved, err := resolved.Effective.Tightened(resolved.Installation, patch); err != nil {
		return domain.Workspace{}, nil, err
	} else if err := w.keepsAWayIn(ctx, stored, patch); err != nil {
		return domain.Workspace{}, nil, err
	} else if len(moved) > 0 || change.RotateNow {
		changed := stored
		changed.Settings.SignIn = stored.Settings.SignIn.Merge(patch)
		// `require_admin_totp` is what `mfa_required_for` derives from, and it is written back so
		// that no client and no stored row has to move (ADR-0068 §1). The boolean is the old name
		// for "ADMINS or stricter".
		if patch.MfaRequiredFor != nil {
			changed.Settings.RequireAdminTotp = *patch.MfaRequiredFor != domain.MfaForNobody
		}
		links, legalMoved, err := w.applyLegal(resolved, changed.Settings.Legal, change.Legal)
		if err != nil {
			return domain.Workspace{}, nil, err
		}
		changed.Settings.Legal = links
		return changed, append(moved, legalMoved...), nil
	}

	links, legalMoved, err := w.applyLegal(resolved, stored.Settings.Legal, change.Legal)
	if err != nil {
		return domain.Workspace{}, nil, err
	}
	if len(legalMoved) == 0 {
		return stored, nil, nil
	}
	changed := stored
	changed.Settings.Legal = links
	return changed, legalMoved, nil
}

// keepsAWayIn refuses switching the password off where no provider is on here: a workspace nobody
// can sign in to is the one arrangement the switches must not reach (UC-ID-12 check 6). Against the
// rule's own field, so the screen can show it at the rule.
func (w WorkspaceWriter) keepsAWayIn(
	ctx context.Context, stored domain.Workspace, patch domain.PolicyPatch,
) error {
	if patch.Methods == nil {
		return nil
	}
	for _, method := range *patch.Methods {
		if method == domain.MethodDirect {
			return nil
		}
	}
	on, err := providerOnHere(ctx, w.Providers, stored.Settings, "", w.Clock.Now())
	if err != nil {
		return err
	}
	if on {
		return nil
	}
	return shared.ErrValidation.
		WithDetail("identity_provider.last_way_in").
		WithFields(shared.FieldError{
			Path: "/sign_in_policy/methods", Code: "identity_provider.last_way_in",
		})
}

// applyLegal folds the four links in, refusing one the instance locked.
func (WorkspaceWriter) applyLegal(
	resolved ResolvedPolicy, stored domain.LegalLinks, sent map[domain.LegalLink]string,
) (domain.LegalLinks, []domain.FieldChange, error) {
	links := stored
	moved := make([]domain.FieldChange, 0, len(sent))

	for _, name := range domain.LegalLinkNames() {
		value, held := sent[name]
		if !held {
			continue
		}
		checked, err := domain.ValidLegalURL(name, value)
		if err != nil {
			return domain.LegalLinks{}, nil, err
		}
		// Against the rule in force rather than against this workspace's own row, because the rule
		// in force is what the screen read and therefore what it sends back. A link inherited from
		// the instance is stored here as nothing at all, so comparing with the stored row makes
		// every inherited link look changed - which is how sending the form back unchanged was
		// refused for a locked link nobody had touched.
		if checked == resolved.Legal.Of(name) {
			// Sent unchanged, which a form does for every field it shows. Not a change, so the
			// lock below has nothing to refuse - see EffectivePolicy.Tightened for the same rule
			// and the same reason. It also means an inherited link is not silently pinned to this
			// workspace by a save that did not touch it.
			continue
		}
		if origin := resolved.LegalLock[name]; origin != domain.LockNone {
			return domain.LegalLinks{}, nil, shared.ErrValidation.
				WithDetail("auth.policy_locked").
				WithParams(map[string]string{"switch": string(name), "origin": string(origin)}).
				WithFields(shared.FieldError{
					Path:   "/sign_in_policy/" + string(name),
					Code:   "auth.policy_locked",
					Params: map[string]string{"origin": string(origin)},
				})
		}
		moved = append(moved, domain.FieldChange{
			Field: string(name), From: resolved.Legal.Of(name), To: checked,
		})
		links = links.With(name, checked)
	}
	return links, moved, nil
}

// signInPolicyOutput is the three-level projection: what is in force, what the level above set, and
// where a lock came from - per switch, because a screen that had to guess between the installation
// and a plan would be a screen telling somebody to ask the wrong person.
func signInPolicyOutput(resolved ResolvedPolicy) usecase.Output {
	policy, above := resolved.Effective.Policy, resolved.Installation

	setting := func(name domain.PolicySwitch, value, installation any) usecase.Output {
		out := usecase.Output{
			"value": value, "installation": installation, "lock": nil,
			// Where the value came from, and who decided the level above it (UC-ID-12 check 4).
			"source":              string(resolved.Effective.SourceOf(name)),
			"installation_source": string(resolved.InstallationEffective.SourceOf(name)),
		}
		if origin := resolved.Effective.LockOf(name); origin != domain.LockNone {
			out["lock"] = string(origin)
		}
		return out
	}

	password := usecase.Output{
		"min_length":       setting(domain.SwitchMinLength, policy.Password.MinLength, above.Password.MinLength),
		"min_lowercase":    setting(domain.SwitchMinLowercase, policy.Password.MinLowercase, above.Password.MinLowercase),
		"min_uppercase":    setting(domain.SwitchMinUppercase, policy.Password.MinUppercase, above.Password.MinUppercase),
		"min_digits":       setting(domain.SwitchMinDigits, policy.Password.MinDigits, above.Password.MinDigits),
		"min_symbols":      setting(domain.SwitchMinSymbols, policy.Password.MinSymbols, above.Password.MinSymbols),
		"min_classes":      setting(domain.SwitchMinClasses, policy.Password.MinClasses, above.Password.MinClasses),
		"max_repeat":       setting(domain.SwitchMaxRepeat, policy.Password.MaxRepeat, above.Password.MaxRepeat),
		"common_passwords": setting(domain.SwitchCommonPasswords, policy.Password.CommonPasswords, above.Password.CommonPasswords),
		"context_words":    setting(domain.SwitchContextWords, policy.Password.ContextWords, above.Password.ContextWords),
		"breach_check":     setting(domain.SwitchBreachCheck, policy.Password.BreachCheck, above.Password.BreachCheck),
		"max_age_days":     setting(domain.SwitchMaxAgeDays, policy.Password.MaxAgeDays, above.Password.MaxAgeDays),
		"history_count":    setting(domain.SwitchHistoryCount, policy.Password.HistoryCount, above.Password.HistoryCount),
		"min_age_hours":    setting(domain.SwitchMinAgeHours, policy.Password.MinAgeHours, above.Password.MinAgeHours),
	}

	legal := usecase.Output{}
	for _, name := range domain.LegalLinkNames() {
		out := usecase.Output{
			"value":        resolved.Legal.Of(name),
			"installation": resolved.InstallationLegal.Of(name),
			"lock":         nil,
			"source": string(domain.LegalSourceOf(
				name, resolved.InstanceLegal, resolved.Workspace.Settings.LegalLayer())),
			"installation_source": string(domain.LegalSourceOf(
				name, resolved.InstanceLegal, domain.LegalLayer{})),
		}
		if origin := resolved.LegalLock[name]; origin != domain.LockNone {
			out["lock"] = string(origin)
		}
		legal[string(name)] = out
	}

	out := usecase.Output{
		"password": password,
		"mfa_required_for": setting(domain.SwitchMfaRequiredFor,
			string(policy.MfaRequiredFor), string(above.MfaRequiredFor)),
		"methods": setting(domain.SwitchMethods,
			methodList(policy.Methods), methodList(above.Methods)),
		"session": usecase.Output{
			"max_days": setting(domain.SwitchSessionMaxDays,
				policy.Sessions.MaxDays, above.Sessions.MaxDays),
			"idle_minutes": setting(domain.SwitchSessionIdleMinutes,
				policy.Sessions.IdleMinutes, above.Sessions.IdleMinutes),
		},
		"legal":         legal,
		"rotation_from": nil,
	}
	if !policy.RotationFrom.IsZero() {
		out["rotation_from"] = policy.RotationFrom.UTC()
	}
	return out
}

func methodList(methods []string) []any {
	values := make([]any, 0, len(methods))
	for _, method := range methods {
		values = append(values, method)
	}
	return values
}

// policyPatchFrom reads the flat patch a client sent into the domain's typed one.
//
// Flat because that is how the settings screen writes it: eighteen controls, eighteen keys, and no
// nesting for a reader to get wrong. Unknown keys are ignored rather than refused - a newer client
// talking to an older server should lose the switch it does not have rather than the save.
func policyPatchFrom(sent map[string]any) (WorkspacePolicyChange, error) {
	change := WorkspacePolicyChange{Legal: map[domain.LegalLink]string{}}

	number := func(key string) (*int, error) {
		raw, held := sent[key]
		if !held || raw == nil {
			return nil, nil
		}
		switch value := raw.(type) {
		case int:
			return &value, nil
		case int64:
			narrowed := int(value)
			return &narrowed, nil
		case float64:
			narrowed := int(value)
			return &narrowed, nil
		}
		return nil, refusedSwitch(key)
	}
	flag := func(key string) (*bool, error) {
		raw, held := sent[key]
		if !held || raw == nil {
			return nil, nil
		}
		value, isBool := raw.(bool)
		if !isBool {
			return nil, refusedSwitch(key)
		}
		return &value, nil
	}

	var err error
	fields := []struct {
		key    string
		target **int
	}{
		{"min_length", &change.Policy.MinLength},
		{"min_lowercase", &change.Policy.MinLowercase},
		{"min_uppercase", &change.Policy.MinUppercase},
		{"min_digits", &change.Policy.MinDigits},
		{"min_symbols", &change.Policy.MinSymbols},
		{"min_classes", &change.Policy.MinClasses},
		{"max_repeat", &change.Policy.MaxRepeat},
		{"max_age_days", &change.Policy.MaxAgeDays},
		{"history_count", &change.Policy.HistoryCount},
		{"min_age_hours", &change.Policy.MinAgeHours},
		{"session_max_days", &change.Policy.SessionMaxDays},
		{"session_idle_minutes", &change.Policy.SessionIdleMinutes},
	}
	for _, field := range fields {
		if *field.target, err = number(field.key); err != nil {
			return WorkspacePolicyChange{}, err
		}
	}
	for _, field := range []struct {
		key    string
		target **bool
	}{
		{"common_passwords", &change.Policy.CommonPasswords},
		{"context_words", &change.Policy.ContextWords},
		{"breach_check", &change.Policy.BreachCheck},
	} {
		if *field.target, err = flag(field.key); err != nil {
			return WorkspacePolicyChange{}, err
		}
	}

	if raw, held := sent["mfa_required_for"]; held && raw != nil {
		value, isString := raw.(string)
		if !isString {
			return WorkspacePolicyChange{}, refusedSwitch("mfa_required_for")
		}
		requirement := domain.MfaRequirement(value)
		if !requirement.Valid() {
			return WorkspacePolicyChange{}, refusedSwitch("mfa_required_for")
		}
		change.Policy.MfaRequiredFor = &requirement
	}
	if raw, held := sent["methods"]; held && raw != nil {
		values, isList := raw.([]any)
		if !isList {
			return WorkspacePolicyChange{}, refusedSwitch("methods")
		}
		methods := make([]string, 0, len(values))
		for _, entry := range values {
			method, isString := entry.(string)
			if !isString {
				return WorkspacePolicyChange{}, refusedSwitch("methods")
			}
			methods = append(methods, method)
		}
		change.Policy.Methods = &methods
	}

	for _, name := range domain.LegalLinkNames() {
		if raw, held := sent[string(name)]; held && raw != nil {
			value, isString := raw.(string)
			if !isString {
				return WorkspacePolicyChange{}, refusedSwitch(string(name))
			}
			change.Legal[name] = value
		}
	}

	if raw, held := sent["rotation_from"]; held && raw != nil {
		// The literal `now` and nothing else. A client that could name a moment could rewind one.
		if value, isString := raw.(string); !isString || value != "now" {
			return WorkspacePolicyChange{}, refusedSwitch("rotation_from")
		}
		change.RotateNow = true
	}

	return change, nil
}

func refusedSwitch(key string) error {
	return shared.ErrValidation.
		WithDetail("auth.policy_value_invalid").
		WithParams(map[string]string{"switch": key}).
		WithFields(shared.FieldError{
			Path: "/sign_in_policy/" + key, Code: "auth.policy_value_invalid",
		})
}
