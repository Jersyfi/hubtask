// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The workspace's own configuration (F4-01).
const (
	readWorkspaceUseCase   = "ReadWorkspace"
	updateWorkspaceUseCase = "UpdateWorkspace"
)

// ReadWorkspace answers GET /tenant.
//
// Written out rather than through the identity helper, for `ListWebhookSubscriptions`' reason:
// the helper's closure takes no context, and an operation with no parameters gives the linter
// nothing to trace the request's context through.
func (c *RestController) ReadWorkspace(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), readWorkspaceUseCase, actorOf(r), usecase.Input{})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusOK, workspaceResponse(out))
}

// UpdateWorkspace answers PATCH /tenant.
func (c *RestController) UpdateWorkspace(
	w http.ResponseWriter, r *http.Request, params openapi.UpdateWorkspaceParams,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		var body openapi.WorkspaceUpdate
		if err := decodeJSON(r, &body); err != nil {
			return nil, err
		}

		in := usecase.Input{
			"display_name":      optionalStringField(body.DisplayName),
			"default_locale":    optionalStringField(body.DefaultLocale),
			"default_time_zone": optionalStringField(body.DefaultTimeZone),
		}
		if body.RequireAdminTotp != nil {
			// Set only when the caller sent one: absent has to reach the catalogue as absent,
			// because false is a value this field legitimately holds.
			in["require_admin_totp"] = *body.RequireAdminTotp
		}
		if version, ok := versionFromIfMatch(params.IfMatch); ok {
			in["expected_version"] = version
		}
		if body.SignInPolicy != nil {
			// The sign-in half travels as a flat document rather than as eighteen declared fields:
			// the catalogue's input is one shape for three channels, and eighteen of them would be
			// eighteen places for a name to drift from the one the domain knows.
			in["sign_in_policy"] = signInPolicyChange(*body.SignInPolicy)
			in["step_up_token"] = stepUpHeaderField(params.XHubtaskStepUp)
		}
		return c.UseCases.Invoke(r.Context(), updateWorkspaceUseCase, actor, in)
	}, func(out usecase.Output) {
		w.Header().Set("ETag", etag(out.Int("version")))
		writeJSON(w, r, http.StatusOK, workspaceResponse(out))
	})
}

// signInPolicyChange flattens the contract's patch into the catalogue's document. Only what the
// caller sent: an absent switch has to reach the use case absent, because zero is a value every one
// of these legitimately holds.
func signInPolicyChange(sent openapi.SignInPolicyChange) map[string]any {
	change := map[string]any{}

	for name, value := range map[string]*int{
		"min_length": sent.MinLength, "min_lowercase": sent.MinLowercase,
		"min_uppercase": sent.MinUppercase, "min_digits": sent.MinDigits,
		"min_symbols": sent.MinSymbols, "min_classes": sent.MinClasses,
		"max_repeat": sent.MaxRepeat, "max_age_days": sent.MaxAgeDays,
		"history_count": sent.HistoryCount, "min_age_hours": sent.MinAgeHours,
		"session_max_days": sent.SessionMaxDays, "session_idle_minutes": sent.SessionIdleMinutes,
	} {
		if value != nil {
			change[name] = *value
		}
	}
	for name, value := range map[string]*bool{
		"common_passwords": sent.CommonPasswords, "context_words": sent.ContextWords,
		"breach_check": sent.BreachCheck,
	} {
		if value != nil {
			change[name] = *value
		}
	}
	for name, value := range map[string]*string{
		"imprint_url": sent.ImprintUrl, "privacy_url": sent.PrivacyUrl,
		"terms_url": sent.TermsUrl, "accessibility_url": sent.AccessibilityUrl,
	} {
		if value != nil {
			change[name] = *value
		}
	}
	if sent.MfaRequiredFor != nil {
		change["mfa_required_for"] = string(*sent.MfaRequiredFor)
	}
	if sent.Methods != nil {
		methods := make([]any, 0, len(*sent.Methods))
		for _, method := range *sent.Methods {
			methods = append(methods, string(method))
		}
		change["methods"] = methods
	}
	if sent.RotationFrom != nil {
		change["rotation_from"] = string(*sent.RotationFrom)
	}
	return change
}

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
		}
	}
	flag := func(document usecase.Output, field string) openapi.SignInPolicyFlag {
		row, _ := document[field].(usecase.Output)
		return openapi.SignInPolicyFlag{
			Value:        boolValue(row["value"]),
			Installation: boolValue(row["installation"]),
			Lock:         policyLock(row["lock"]),
		}
	}
	word := func(document usecase.Output, field string) openapi.SignInPolicyText {
		row, _ := document[field].(usecase.Output)
		return openapi.SignInPolicyText{
			Value:        textValue(row["value"]),
			Installation: textValue(row["installation"]),
			Lock:         policyLock(row["lock"]),
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
			Value:        methodEnums(methods["value"]),
			Installation: methodEnums(methods["installation"]),
			Lock:         policyLock(methods["lock"]),
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
		},
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
