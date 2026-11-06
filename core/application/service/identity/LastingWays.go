// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"slices"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	stepupport "github.com/Jersyfi/hubtask/core/port/stepup"
)

// LastingWays answers what one person would still sign in with after a change of the workspace's
// address (multi-tenancy.md §3.1): the step-up's reading of the ways an account holds - a password
// the workspace lets it use, a connected provider switched on there, the app, recovery codes and
// passkeys - and the rule's demand for a factor, the one reading signing in makes. The control plane
// asks it for the person making the change and for the workspace's owners.
type LastingWays struct{ Writer SessionWriter }

// WaysOf reads them in the person's own workspace.
func (l LastingWays) WaysOf(ctx context.Context, tenantID, accountID shared.ID) (domain.WaysHeld, error) {
	methods, err := StepUpVerifier(l).Methods(ctx, tenantID, accountID)
	if err != nil {
		return domain.WaysHeld{}, err
	}
	required, err := l.Writer.DemandsFactorOf(ctx, appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantID, AccountID: accountID,
	})
	if err != nil {
		return domain.WaysHeld{}, err
	}
	return domain.WaysHeld{
		Password:       slices.Contains(methods, stepupport.MethodPassword),
		Provider:       slices.Contains(methods, stepupport.MethodProvider),
		App:            slices.Contains(methods, stepupport.MethodTotp),
		RecoveryCodes:  slices.Contains(methods, stepupport.MethodRecovery),
		Passkeys:       slices.Contains(methods, stepupport.MethodPasskey),
		FactorRequired: required,
	}, nil
}
