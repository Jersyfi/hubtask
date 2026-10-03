// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"time"

	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domainidentity "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// Withdrawing an offered provider (ADR-0076 §2-3, UC-INS-11 check 5).
//
// **A number, a date, and a way back.** The operator sees how many workspaces use the provider -
// never which (P-01) - and names when the offer ends, fourteen days ahead by default. Until then it
// keeps working and the workspaces that use it say when it ends; the withdrawal can be cancelled at
// any time, after the date too, because the identities it connected were never removed. *Withdraw
// now* is the same act with a date that has come, behind the same step-up and a confirmation that
// repeats the count: the answer to a compromised provider.

const (
	WithdrawInstanceIdentityProviderName         = "WithdrawInstanceIdentityProvider"
	CancelInstanceIdentityProviderWithdrawalName = "CancelInstanceIdentityProviderWithdrawal"

	instanceProviderWithdrawnAction           audit.Action = "instance.provider_withdrawn"
	instanceProviderWithdrawalCancelledAction audit.Action = "instance.provider_withdrawal_cancelled"

	journalProviderWithdrawn           = string(instanceProviderWithdrawnAction)
	journalProviderWithdrawalCancelled = string(instanceProviderWithdrawalCancelledAction)
)

// WithdrawCommand is the input, typed. A zero At is the default notice; one that has come is now.
type WithdrawCommand struct {
	ID           shared.ID
	At           time.Time
	ConfirmCount *int
	StepUpToken  string
}

// WithdrawInstanceIdentityProvider announces the end of an offer, or ends it now.
type WithdrawInstanceIdentityProvider struct{ Writer InstanceProviderWriter }

// Execute withdraws.
func (h WithdrawInstanceIdentityProvider) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd WithdrawCommand,
) (domainidentity.IdentityProvider, error) {
	w := h.Writer
	if err := w.Instance.authorize(ctx, actor); err != nil {
		return domainidentity.IdentityProvider{}, err
	}
	stored, err := w.Configure.WithdrawOfferAt(
		ctx, persistence.SystemScope(), actor, cmd.ID, cmd.At, cmd.ConfirmCount, cmd.StepUpToken)
	if err != nil {
		return domainidentity.IdentityProvider{}, err
	}
	// The installation's own evidence: the date and the number it was withdrawn under. A number,
	// never a workspace - the journal is read by people the workspaces never chose.
	if err := w.record(ctx, actor, journalProviderWithdrawn, map[string]any{
		"provider_id":        stored.ID.String(),
		"withdraw_at":        stored.WithdrawAt.UTC().Format(time.RFC3339),
		"offered_workspaces": stored.OfferedWorkspaces,
	}); err != nil {
		return domainidentity.IdentityProvider{}, err
	}
	return stored, nil
}

// CancelInstanceIdentityProviderWithdrawal keeps offering it.
type CancelInstanceIdentityProviderWithdrawal struct{ Writer InstanceProviderWriter }

// Execute cancels.
func (h CancelInstanceIdentityProviderWithdrawal) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID, stepUpToken string,
) (domainidentity.IdentityProvider, error) {
	w := h.Writer
	if err := w.Instance.authorize(ctx, actor); err != nil {
		return domainidentity.IdentityProvider{}, err
	}
	stored, err := w.Configure.CancelWithdrawalAt(
		ctx, persistence.SystemScope(), actor, id, stepUpToken)
	if err != nil {
		return domainidentity.IdentityProvider{}, err
	}
	if err := w.record(ctx, actor, journalProviderWithdrawalCancelled, map[string]any{
		"provider_id":        stored.ID.String(),
		"offered_workspaces": stored.OfferedWorkspaces,
	}); err != nil {
		return domainidentity.IdentityProvider{}, err
	}
	return stored, nil
}

// InstanceProviderOutput is the operator's projection: a workspace's, and the count beside it. The
// count is in this one and in no other, because only the installation may know it (ADR-0076 §1).
func InstanceProviderOutput(configured domainidentity.IdentityProvider) usecase.Output {
	out := identityservice.ProviderOutput(configured)
	out["offered_workspaces"] = configured.OfferedWorkspaces
	return out
}

func (h WithdrawInstanceIdentityProvider) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: WithdrawInstanceIdentityProviderName,
		Summary: "Announces the end of a provider this installation offers every workspace, or " +
			"ends it now. Absent a date it ends fourteen days from now; until then it keeps working " +
			"and every workspace that uses it says when it ends. A date that has come is Withdraw " +
			"now, the answer to a compromised provider, and must repeat the number of workspaces " +
			"that have it switched on. The connected identities stay, so cancelling restores sign-in.",
		SideEffects: "Writes the withdrawal date and one journal entry carrying the date and the count.",
		TokenScope:  adminTenantsScope,
		Input: []usecase.Field{
			{Name: "id", Kind: usecase.KindString, Required: true,
				Description: "The provider to withdraw."},
			{Name: "withdraw_at", Kind: usecase.KindString, Format: usecase.FormatDateTime,
				Description: "When the offer ends, RFC 3339. Absent is fourteen days from now; now or past is Withdraw now."},
			{Name: "confirm_count", Kind: usecase.KindInt,
				Description: "Required for Withdraw now: the number of workspaces that have the provider switched on, as just read."},
			identityservice.ProviderStepUpField,
		},
		StepUp: "changing a way in every workspace is offered",
		Audit: usecase.AuditDeclaration{
			Action: instanceProviderWithdrawnAction, TargetType: instanceProviderTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "The installation's configuration is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h WithdrawInstanceIdentityProvider) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("id")
	if err != nil {
		return nil, err
	}
	cmd := WithdrawCommand{ID: id, StepUpToken: in.String("step_up_token")}
	if raw := in.String("withdraw_at"); raw != "" {
		at, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return nil, shared.ErrValidation.
				WithDetail("identity_provider.withdraw_at_malformed").
				WithParams(map[string]string{"value": raw}).
				WithFields(shared.FieldError{Path: "/withdraw_at", Code: "identity_provider.withdraw_at_malformed"})
		}
		cmd.At = at
	}
	if in.Present("confirm_count") {
		count := in.Int("confirm_count")
		cmd.ConfirmCount = &count
	}
	stored, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return InstanceProviderOutput(stored), nil
}

func (h CancelInstanceIdentityProviderWithdrawal) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CancelInstanceIdentityProviderWithdrawalName,
		Summary: "Keeps offering a provider whose withdrawal was announced. Before the date nothing " +
			"changes for anybody; after it, the workspaces that had it switched on sign in through " +
			"it again, because neither their switches nor the connected identities were removed.",
		SideEffects: "Clears the withdrawal date and writes one journal entry.",
		TokenScope:  adminTenantsScope,
		Input: []usecase.Field{
			{Name: "id", Kind: usecase.KindString, Required: true,
				Description: "The provider to keep offering."},
			identityservice.ProviderStepUpField,
		},
		StepUp: "changing a way in every workspace is offered",
		Audit: usecase.AuditDeclaration{
			Action: instanceProviderWithdrawalCancelledAction, TargetType: instanceProviderTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "The installation's configuration is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h CancelInstanceIdentityProviderWithdrawal) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("id")
	if err != nil {
		return nil, err
	}
	stored, err := h.Execute(ctx, actor, id, in.String("step_up_token"))
	if err != nil {
		return nil, err
	}
	return InstanceProviderOutput(stored), nil
}
