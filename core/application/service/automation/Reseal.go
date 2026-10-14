// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"encoding/base64"
	"strings"

	repository "github.com/Jersyfi/hubtask/core/application/repository/automation"
	sealingrepo "github.com/Jersyfi/hubtask/core/application/repository/sealing"
	"github.com/Jersyfi/hubtask/core/application/service/sealing"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
)

// RuleResealer moves the header secrets of a workspace's HTTP_REQUEST actions under the current
// master key (ADR-0045). The one store where the sealed values live inside a document rather
// than in a column: the walk is sealActions' walk, so a secret at any depth of a branch is found
// the way it was sealed, and the purpose is ruleSecretPurpose's - the rule's, never the stored
// string's, so a document that was tampered with cannot name its own purpose.
type RuleResealer struct {
	Rules     repository.RuleSealings
	Encryptor crypto.Encryptor
}

var _ sealing.Resealer = RuleResealer{}

func (RuleResealer) Store() string { return "automation_rule" }

func (r RuleResealer) Reseal(ctx context.Context, _ shared.ID) (sealing.Outcome, error) {
	var outcome sealing.Outcome
	active := r.Encryptor.ActiveKeyID()
	rules, err := r.Rules.SealedNotUnder(ctx, active)
	if err != nil {
		return outcome, err
	}
	for _, rule := range rules {
		var moved, stuck int64
		err := walkOutbound(rule.Actions, "", func(path string, params map[string]any) error {
			request, err := domain.ReadHTTPRequest(params, path)
			if err != nil || request.Sealed == nil || request.Sealed.KeyID == active {
				return nil
			}
			ciphertext, err := base64.StdEncoding.DecodeString(request.Sealed.Ciphertext)
			if err != nil {
				// A document this build cannot read is a document it must not rewrite.
				stuck++
				return nil
			}
			rewrapped, err := r.Encryptor.Rewrap(ctx,
				crypto.Sealed{KeyID: request.Sealed.KeyID, Ciphertext: ciphertext},
				ruleSecretPurpose(rule.ID))
			if err != nil {
				if sealing.Unopenable(err) {
					stuck++
					return nil
				}
				return err
			}
			params["secret_header_sealed"] = domain.SealedSecret{
				Ciphertext: base64.StdEncoding.EncodeToString(rewrapped.Ciphertext),
				KeyID:      rewrapped.KeyID,
				Purpose:    string(ruleSecretPurpose(rule.ID)),
			}.Document()
			moved++
			return nil
		})
		if err != nil {
			return outcome, err
		}
		outcome.Skipped += stuck
		if moved == 0 {
			continue
		}
		written, err := r.Rules.RewrapActions(ctx, rule.ID, rule.Actions, rule.Version)
		if err != nil {
			return outcome, err
		}
		if written {
			outcome.Rewrapped += moved
		}
	}
	return outcome, nil
}

// CallResealer moves the header secrets of a workspace's HTTP calls still waiting to be made or
// retried under the current master key (ADR-0045). The secret lives in the job's payload until the
// job ends: a direct call's sealed under the request's purpose, a rule's action's carried over from
// the rule under the rule's.
//
// The payload may be shaped by a caller, so neither its purpose nor its ciphertext is taken on
// trust: a purpose of neither shape is not attempted, and a ciphertext the envelope refuses under
// the purpose it names is skipped and counted rather than failing the workspace's round. The
// purpose is bound into the encryption, so a rewrap under a purpose the value was not sealed for
// cannot succeed; the shape check only keeps the round from attempting what no job of this
// system writes.
type CallResealer struct {
	Calls     repository.CallSealings
	Encryptor crypto.Encryptor
}

var _ sealing.Resealer = CallResealer{}

func (CallResealer) Store() string { return sealingrepo.StoreAutomationHTTPJob }

func (r CallResealer) Reseal(ctx context.Context, _ shared.ID) (sealing.Outcome, error) {
	var outcome sealing.Outcome
	active := r.Encryptor.ActiveKeyID()
	calls, err := r.Calls.SealedNotUnder(ctx, active)
	if err != nil {
		return outcome, err
	}
	for _, call := range calls {
		purpose, known := callSecretPurpose(call)
		ciphertext, err := base64.StdEncoding.DecodeString(call.Sealed.Ciphertext)
		if !known || err != nil || call.Sealed.KeyID == "" {
			outcome.Skipped++
			continue
		}
		rewrapped, err := r.Encryptor.Rewrap(ctx,
			crypto.Sealed{KeyID: call.Sealed.KeyID, Ciphertext: ciphertext}, purpose)
		if err != nil {
			if sealing.Unopenable(err) || refusedAsForeign(err) {
				outcome.Skipped++
				continue
			}
			return outcome, err
		}
		written, err := r.Calls.RewrapCall(ctx, call.JobID, domain.SealedSecret{
			Ciphertext: base64.StdEncoding.EncodeToString(rewrapped.Ciphertext),
			KeyID:      rewrapped.KeyID,
			Purpose:    string(purpose),
		}, call.Sealed.KeyID)
		if err != nil {
			return outcome, err
		}
		if written {
			outcome.Rewrapped++
		}
	}
	return outcome, nil
}

// callSecretPurpose answers the purpose a call job's secret was sealed under, when it has one of
// the two shapes this system writes: the request's own, or a rule's carried over from the rule.
func callSecretPurpose(call repository.PendingCall) (crypto.Purpose, bool) {
	stored := call.Sealed.Purpose
	if requestID, err := shared.ParseID(call.RequestID); err == nil &&
		stored == string(requestSecretPurpose(requestID)) {
		return crypto.Purpose(stored), true
	}
	if ruleID, found := strings.CutPrefix(stored, string(ruleSecretPurpose(""))); found {
		if id, err := shared.ParseID(ruleID); err == nil {
			return ruleSecretPurpose(id), true
		}
	}
	return "", false
}

// refusedAsForeign reports the envelope's refusals of a ciphertext that was not sealed for the
// purpose it names, or is not an envelope at all: a document a caller shaped, which this round
// leaves where it is. It drains with its job, and the census counts it until then.
func refusedAsForeign(err error) bool {
	code := shared.AsError(err).DetailCode
	return code == crypto.CodeNotAuthentic || code == crypto.CodeCiphertextMalformed
}
