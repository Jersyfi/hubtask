// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
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
)

func invalid(code, field string) error {
	return shared.ErrValidation.
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

// bounded trims and refuses text longer than a column should carry.
func bounded(value string, limit int, code, field string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if len([]rune(trimmed)) > limit {
		return "", invalid(code, field)
	}
	return trimmed, nil
}
