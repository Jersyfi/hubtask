// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The passkeys of the caller's own account (identity.md §23).

const (
	completePasskeyRegistrationUseCase = "CompletePasskeyRegistration"
)

// passkeyListResponse maps the caller's passkeys with what they may do with them.
func passkeyListResponse(out usecase.Output) openapi.PasskeyList {
	rows, _ := out["data"].([]usecase.Output)
	answer := openapi.PasskeyList{
		Passkeys: make([]openapi.Passkey, 0, len(rows)),
		CanAdd:   out["can_add"] == true, CanRemove: out["can_remove"] == true,
	}
	answer.Limit, _ = out["limit"].(int)
	if rpID := out.String("rp_id"); rpID != "" {
		answer.RpId = &rpID
	}
	if reason := out.String("cannot_add_reason"); reason != "" {
		code := openapi.PasskeyListCannotAddReason(reason)
		answer.CannotAddReason = &code
	}
	if reason := out.String("cannot_remove_reason"); reason != "" {
		code := openapi.PasskeyListCannotRemoveReason(reason)
		answer.CannotRemoveReason = &code
	}
	for _, row := range rows {
		answer.Passkeys = append(answer.Passkeys, passkeyResponse(row))
	}
	return answer
}

func passkeyResponse(out usecase.Output) openapi.Passkey {
	id, _ := uuid.Parse(out.String("id"))
	answer := openapi.Passkey{
		Id: id, Name: out.String("name"),
		CreatedAt: timeValue(out["created_at"]), LastUsedAt: timePointer(out["last_used_at"]),
	}
	if rpID := out.String("rp_id"); rpID != "" {
		answer.RpId = &rpID
	}
	return answer
}

// renamedPasskeyResponse maps the passkey a rename answers.
func renamedPasskeyResponse(out usecase.Output) openapi.Passkey {
	passkey, _ := out["passkey"].(usecase.Output)
	return passkeyResponse(passkey)
}

// CompletePasskeyRegistration answers POST /accounts/me/passkeys:complete.
func (c *RestController) CompletePasskeyRegistration(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	var body openapi.PasskeyRegistration
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	in := usecase.Input{
		"name": body.Name, "raw_id": body.RawId, "client_data_json": body.ClientDataJson,
		"attestation_object": body.AttestationObject,
	}
	if body.Transports != nil {
		transports := make([]any, 0, len(*body.Transports))
		for _, transport := range *body.Transports {
			transports = append(transports, string(transport))
		}
		in["transports"] = transports
	}
	out, err := c.UseCases.Invoke(r.Context(), completePasskeyRegistrationUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	passkey, _ := out["passkey"].(usecase.Output)
	answer := openapi.PasskeyRegistered{Passkey: passkeyResponse(passkey)}
	if codes, ok := out["recovery_codes"].([]any); ok && len(codes) > 0 {
		// The codes are in this answer only for the account's first factor; the mark keeps them out
		// of the idempotency record (ShownOnceByHandler).
		w.Header().Set(ShownOnceHeader, "recovery_codes")
		listed := make([]string, 0, len(codes))
		for _, code := range codes {
			text, _ := code.(string)
			listed = append(listed, text)
		}
		answer.RecoveryCodes = &listed
	}
	writeJSON(w, r, http.StatusCreated, answer)
}

// The passkey sign-in (identity.md §23.4). Written out, SignIn's reasoning: the routes are public, so
// there is no actor to resolve - the call exists to produce one - and the request's host, client and
// workspace sources travel as declared inputs, because only this adapter has the request.

const (
	completePasskeySignInUseCase = "CompletePasskeySignIn"
)

func (c *RestController) passkeySignInInput(r *http.Request) usecase.Input {
	return usecase.Input{
		"host": r.Host, "tenant_header": r.Header.Get(TenantHeader),
		"user_agent": r.UserAgent(), "remote_addr": r.RemoteAddr,
	}
}

// CompletePasskeySignIn answers POST /auth/sessions:passkey.
func (c *RestController) CompletePasskeySignIn(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	var body openapi.PasskeyAssertion
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	in := c.passkeySignInInput(r)
	in["raw_id"], in["client_data_json"] = body.RawId, body.ClientDataJson
	in["authenticator_data"], in["signature"], in["user_handle"] = body.AuthenticatorData, body.Signature, body.UserHandle
	out, err := c.UseCases.Invoke(r.Context(), completePasskeySignInUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	if required, _ := out["mfa_required"].(bool); required {
		writeJSON(w, r, http.StatusAccepted, mfaChallengeResponse(out))
		return
	}
	writeJSON(w, r, http.StatusCreated, sessionTokensResponse(out))
}

const (
	startPasskeyEnrollmentUseCase    = "StartPasskeyEnrollment"
	completePasskeyEnrollmentUseCase = "CompletePasskeyEnrollment"
)

// StartPasskeyEnrollment answers POST /auth/mfa/passkey:enroll-options. Public, for ConfirmTotp's
// reason: the ENROLL credential in the body is what authenticates the call.
func (c *RestController) StartPasskeyEnrollment(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	var body openapi.PasskeyEnrollmentStart
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	out, err := c.UseCases.Invoke(r.Context(), startPasskeyEnrollmentUseCase, actorOf(r), usecase.Input{
		"pending_token": body.PendingToken, "host": r.Host,
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	var answer openapi.PasskeyRegistrationOptions
	encoded, err := json.Marshal(out)
	if err == nil {
		err = json.Unmarshal(encoded, &answer)
	}
	if err != nil {
		WriteProblem(w, shared.ErrInternal.WithDetail("rest.answer_unshapeable").WithCause(err), requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, answer)
}

// CompletePasskeyEnrollment answers POST /auth/mfa/passkey:enroll: the key kept, and the sign-in's
// pair or the step it still owes.
func (c *RestController) CompletePasskeyEnrollment(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	var body openapi.PasskeyEnrollment
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	in := usecase.Input{
		"pending_token": body.PendingToken, "name": body.Name, "raw_id": body.RawId,
		"client_data_json": body.ClientDataJson, "attestation_object": body.AttestationObject,
	}
	if body.Transports != nil {
		transports := make([]any, 0, len(*body.Transports))
		for _, transport := range *body.Transports {
			transports = append(transports, string(transport))
		}
		in["transports"] = transports
	}
	out, err := c.UseCases.Invoke(r.Context(), completePasskeyEnrollmentUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	passkey, _ := out["passkey"].(usecase.Output)
	answer := openapi.PasskeyEnrolled{Passkey: passkeyResponse(passkey)}
	if codes, ok := out["recovery_codes"].([]any); ok && len(codes) > 0 {
		listed := make([]string, 0, len(codes))
		for _, code := range codes {
			text, _ := code.(string)
			listed = append(listed, text)
		}
		answer.RecoveryCodes = &listed
	}
	if tokens, ok := out["tokens"].(usecase.Output); ok {
		pair := sessionTokensResponse(tokens)
		answer.Tokens = &pair
	}
	if challenge, ok := out["challenge"].(usecase.Output); ok {
		owed := mfaChallengeResponse(challenge)
		answer.Challenge = &owed
	}
	writeJSON(w, r, http.StatusCreated, answer)
}
