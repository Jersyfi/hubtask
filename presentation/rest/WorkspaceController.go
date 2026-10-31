// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The workspace's own configuration.
const (
	readWorkspaceUseCase = "ReadWorkspace"
)

// workspaceResponse maps the use case's answer.
func workspaceResponse(out usecase.Output) openapi.Workspace {
	enforced, _ := out["require_admin_totp"].(bool)
	answer := openapi.Workspace{
		Id:               uuidValue(out.String("id")),
		Slug:             out.String("slug"),
		DisplayName:      out.String("display_name"),
		Status:           openapi.WorkspaceStatus(out.String("status")),
		DefaultLocale:    out.String("default_locale"),
		DefaultTimeZone:  out.String("default_time_zone"),
		RequireAdminTotp: enforced,
		CreatedAt:        timeValue(out["created_at"]),
		Version:          out.Int("version"),
	}
	if several, held := out["several_people"].(bool); held {
		answer.SeveralPeople = &several
	}
	if offered, held := out["private_hubs_offered"].(bool); held {
		answer.PrivateHubsOffered = &offered
	}
	if updated, held := out["updated_at"].(time.Time); held && !updated.IsZero() {
		answer.UpdatedAt = &updated
	}
	if target := out.String("audit_anchor_target_id"); target != "" {
		id := uuidValue(target)
		answer.AuditAnchorTargetId = &id
	}
	if policy, held := out["sign_in_policy"].(usecase.Output); held {
		answer.SignInPolicy = signInPolicyResponse(policy)
	}
	if structure, held := out["structure"]; held {
		var answered openapi.WorkspaceStructure
		if throughJSON(structure, &answered) == nil {
			answer.Structure = &answered
		}
	}
	if day := out.String("default_week_start"); day != "" {
		weekStart := openapi.WorkspaceDefaultWeekStart(day)
		answer.DefaultWeekStart = &weekStart
	}
	if defaults, held := out["installation_defaults"].(usecase.Output); held {
		installation := openapi.InstallationDefaults{
			Locale:   optionalStringOutput(defaults, "locale"),
			TimeZone: optionalStringOutput(defaults, "time_zone"),
		}
		if day := defaults.String("week_start"); day != "" {
			weekStart := openapi.InstallationDefaultsWeekStart(day)
			installation.WeekStart = &weekStart
		}
		answer.InstallationDefaults = &installation
	}
	if opening, held := out["password_opening"].(usecase.Output); held {
		// Who asked and why travel only where the use case answered them (READ_CONFIGURATION).
		answer.PasswordOpening = &openapi.WorkspacePasswordOpening{
			Until:     timeValue(opening["until"]),
			Requester: optionalStringOutput(opening, "requester"),
			Reason:    optionalStringOutput(opening, "reason"),
		}
	}
	if rows, held := out["hosts"].([]usecase.Output); held && len(rows) > 0 {
		hosts := make([]openapi.WorkspaceHost, 0, len(rows))
		for _, row := range rows {
			host := openapi.WorkspaceHost{
				Host:        row.String("host"),
				State:       openapi.WorkspaceHostState(row.String("state")),
				IsCanonical: boolValue(row["is_canonical"]),
				CreatedAt:   timeValue(row["created_at"]),
			}
			if mark := row.String("verification"); mark != "" {
				host.Verification = &mark
			}
			if verified, ok := row["verified_at"].(time.Time); ok && !verified.IsZero() {
				host.VerifiedAt = &verified
			}
			hosts = append(hosts, host)
		}
		answer.Hosts = &hosts
	}
	return answer
}

// signInPolicyResponse maps the three-level projection. One helper per kind of switch, because the
// contract has a type per kind: a number and a flag are different documents on the wire even where
// they are the same idea.
func signInPolicyResponse(out usecase.Output) *openapi.SignInPolicy {
	number := func(document usecase.Output, field string) openapi.SignInPolicyNumber {
		row, _ := document[field].(usecase.Output)
		return openapi.SignInPolicyNumber{
			Value:        intValue(row["value"]),
			Installation: intValue(row["installation"]),
			Lock:         policyLock(row["lock"]),
			Source:       policySource(row["source"]),
			// Who decided the level above: the screen names the installation only where it did.
			InstallationSource: policySource(row["installation_source"]),
		}
	}
	flag := func(document usecase.Output, field string) openapi.SignInPolicyFlag {
		row, _ := document[field].(usecase.Output)
		return openapi.SignInPolicyFlag{
			Value:        boolValue(row["value"]),
			Installation: boolValue(row["installation"]),
			Lock:         policyLock(row["lock"]),
			Source:       policySource(row["source"]),
			// Who decided the level above: the screen names the installation only where it did.
			InstallationSource: policySource(row["installation_source"]),
		}
	}
	word := func(document usecase.Output, field string) openapi.SignInPolicyText {
		row, _ := document[field].(usecase.Output)
		return openapi.SignInPolicyText{
			Value:        textValue(row["value"]),
			Installation: textValue(row["installation"]),
			Lock:         policyLock(row["lock"]),
			Source:       policySource(row["source"]),
			// Who decided the level above: the screen names the installation only where it did.
			InstallationSource: policySource(row["installation_source"]),
		}
	}

	password, _ := out["password"].(usecase.Output)
	session, _ := out["session"].(usecase.Output)
	legal, _ := out["legal"].(usecase.Output)
	methods, _ := out["methods"].(usecase.Output)

	answer := openapi.SignInPolicy{
		Password: openapi.PasswordPolicySettings{
			MinLength:       number(password, "min_length"),
			MinLowercase:    number(password, "min_lowercase"),
			MinUppercase:    number(password, "min_uppercase"),
			MinDigits:       number(password, "min_digits"),
			MinSymbols:      number(password, "min_symbols"),
			MinClasses:      number(password, "min_classes"),
			MaxRepeat:       number(password, "max_repeat"),
			MaxAgeDays:      number(password, "max_age_days"),
			HistoryCount:    number(password, "history_count"),
			MinAgeHours:     number(password, "min_age_hours"),
			CommonPasswords: flag(password, "common_passwords"),
			ContextWords:    flag(password, "context_words"),
			BreachCheck:     flag(password, "breach_check"),
		},
		MfaRequiredFor: word(out, "mfa_required_for"),
		Methods: openapi.SignInPolicyMethods{
			Value:              methodEnums(methods["value"]),
			Installation:       methodEnums(methods["installation"]),
			Lock:               policyLock(methods["lock"]),
			Source:             policySource(methods["source"]),
			InstallationSource: policySource(methods["installation_source"]),
		},
		Session: openapi.SessionPolicySettings{
			MaxDays:     number(session, "max_days"),
			IdleMinutes: number(session, "idle_minutes"),
		},
		Legal: openapi.LegalPolicySettings{
			ImprintUrl:       word(legal, "imprint_url"),
			PrivacyUrl:       word(legal, "privacy_url"),
			TermsUrl:         word(legal, "terms_url"),
			AccessibilityUrl: word(legal, "accessibility_url"),
			TermsVersion:     word(legal, "terms_version"),
		},
	}
	if _, held := out["managed_accounts"].(usecase.Output); held {
		managed := flag(out, "managed_accounts")
		answer.ManagedAccounts = &managed
	}
	if plan := out.String("plan"); plan != "" {
		answer.Plan = &plan
	}
	if _, held := out["private_hubs"].(usecase.Output); held {
		allowed := flag(out, "private_hubs")
		answer.PrivateHubs = &allowed
	}
	if moment, held := out["rotation_from"].(time.Time); held && !moment.IsZero() {
		answer.RotationFrom = &moment
	}
	return &answer
}

func textValue(value any) string {
	text, _ := value.(string)
	return text
}

// policyLock maps the origin, or nothing where the switch is open. The origin rather than a boolean,
// because "ask your administrator" and "ask your provider" are different sentences (ADR-0070 §3).
func policyLock(value any) *openapi.PolicyLock {
	origin, isString := value.(string)
	if !isString || origin == "" {
		return nil
	}
	lock := openapi.PolicyLock(origin)
	return &lock
}

// policySource maps where a value came from (UC-ID-12 check 4). A row that says nothing is the
// product's default, which is the honest reading of "no level decided it".
func policySource(value any) openapi.PolicySource {
	if source, isString := value.(string); isString && source != "" {
		return openapi.PolicySource(source)
	}
	return openapi.PolicySourceDEFAULT
}

func methodEnums(value any) []openapi.SignInMethod {
	values, _ := value.([]any)
	methods := make([]openapi.SignInMethod, 0, len(values))
	for _, entry := range values {
		if method, isString := entry.(string); isString {
			methods = append(methods, openapi.SignInMethod(method))
		}
	}
	return methods
}

// optionalStringOutput is a string field of an answer, nil where the answer left it out.
func optionalStringOutput(out usecase.Output, field string) *string {
	value, held := out[field].(string)
	if !held || value == "" {
		return nil
	}
	return &value
}
