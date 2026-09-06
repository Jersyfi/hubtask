// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"encoding/base64"

	repository "github.com/Jersyfi/hubtask/core/application/repository/automation"
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
