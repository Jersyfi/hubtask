// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// The message codes of this context. Codes rather than sentences (ADR-0011): what a person reads
// is rendered from `locales/en.json`, and a refusal an operator has to act on - "this case cannot
// start without a mode" - has to say which field it is about.
const (
	CodeKindInvalid             = "privacy.kind_invalid"
	CodeScopeInvalid            = "privacy.scope_invalid"
	CodeSubjectRequired         = "privacy.subject_required"
	CodeDeadlineInPast          = "privacy.deadline_in_past"
	CodeNotesTooLong            = "privacy.notes_too_long"
	CodeReasonTooLong           = "privacy.reason_too_long"
	CodeRejectionReasonRequired = "privacy.rejection_reason_required"
	CodeErasureModeRequired     = "privacy.erasure_mode_required"
	CodeExportTargetRequired    = "privacy.export_target_required"
	CodeTransitionRefused       = "privacy.transition_refused"
	CodeRequestNotFound         = "privacy.request_not_found"
	CodeRequestClosed           = "privacy.request_closed"
	CodePurposeRequired         = "privacy.purpose_required"
	CodeConsentNotFound         = "privacy.consent_not_found"
	CodeSubjectNotFound         = "privacy.subject_not_found"
	CodeInstallationScopeDenied = "privacy.installation_scope_denied"
	// CodeErasureBlockedByRule is the refusal PG-2 asked for: the person still runs automation,
	// and a rule that acts as somebody must not start acting as nobody. The operator re-points or
	// removes the rules, and the case is carried out afterwards.
	CodeErasureBlockedByRule = "privacy.erasure_blocked_by_rule"
	// CodeRestrictionKeptByErasure is lifting the restriction of an account a legal hold keeps
	// from an erasure: the restriction stands until the rest of the erasure has run.
	CodeRestrictionKeptByErasure = "privacy.restriction_kept_by_erasure"
	// CodeNotAnErasure is asking what an erasure would keep of a case that is not one.
	CodeNotAnErasure = "privacy.not_an_erasure"

	// The extension's refusals (data-protection.md §4.1). CodeExtensionReasonInvalid guards a
	// caller below the registry, which refuses an unknown reason by its enum first.
	CodeAlreadyExtended        = "privacy.already_extended"
	CodeExtensionAfterDeadline = "privacy.extension_after_deadline"
	CodeExtensionTooLong       = "privacy.extension_too_long"
	CodeExtensionNotLater      = "privacy.extension_not_later"
	CodeInformedOnOutOfRange   = "privacy.informed_on_out_of_range"
	CodeExtensionReasonInvalid = "privacy.extension_reason_invalid"
)

func invalid(code, field string) error {
	return shared.ErrValidation.
		WithDetail(code).
		WithFields(shared.FieldError{Path: field, Code: code})
}

func invalidWith(code, field string, params map[string]string) error {
	return shared.ErrValidation.
		WithDetail(code).
		WithParams(params).
		WithFields(shared.FieldError{Path: field, Code: code})
}

// conflict is a refusal about the case's state rather than the input: the same request may succeed
// against another case, never against this one.
func conflict(code, field string) error {
	return shared.ErrConflict.
		WithDetail(code).
		WithFields(shared.FieldError{Path: field, Code: code})
}

// transitionRefused names both ends of the step that was refused, because "that is not allowed" is
// not something an operator can act on and "RECEIVED cannot become COMPLETED" is.
func transitionRefused(from, to Status) error {
	return shared.ErrConflict.
		WithDetail(CodeTransitionRefused).
		WithParams(map[string]string{"from": string(from), "to": string(to)}).
		WithFields(shared.FieldError{Path: "/status", Code: CodeTransitionRefused})
}

// bounded trims, brings to normal form C (i18n-l10n.md §5), and refuses text longer than a column
// should carry.
func bounded(value string, limit int, code, field string, form text.Normalizer) (string, error) {
	trimmed, err := shared.NFC(strings.TrimSpace(value), form)
	if err != nil {
		return "", err
	}
	if len([]rune(trimmed)) > limit {
		return "", invalid(code, field)
	}
	return trimmed, nil
}
