// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// LocateConnectorReturnName is the catalogue's name for the lookup.
const LocateConnectorReturnName = "LocateConnectorReturn"

// The connections whose provider returns to a page of the installation's.
const (
	ConnectorMailbox      = "MAILBOX"
	ConnectorCalendarPush = "CALENDAR_PUSH"
	ConnectorFileStore    = "FILE_STORE"
)

// AwaitedReturns reads, in the transaction's workspace, whether a connection still waits for its
// provider's answer under a state's hash - without spending it.
type AwaitedReturns interface {
	Live(ctx context.Context, stateHash []byte, now time.Time) (bool, error)
}

// LocateConnectorReturn answers where a mailbox's, a calendar push's or a file store's provider
// return belongs (multi-tenancy.md §3) - LocateOidcFlow's counterpart for the connectors.
//
// The provider returns to the installation's one registered page, which in multi-tenant operation
// is on the installation's host, while the connection began on the workspace's host, where the
// session that completes it lives. The state names the workspace (shared.ReturnState); the
// workspace part proves nothing, so the address is answered only when, in that workspace, a
// connection of that kind still waits under the state's hash. Anything else is one refusal that
// names no workspace.
type LocateConnectorReturn struct {
	// Awaited holds each connector's waiting connections by kind.
	Awaited    map[string]AwaitedReturns
	Address    WorkspaceAddress
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

func connectorReturnNotFound() error {
	return shared.ErrNotFound.WithDetail("auth.connector_return_not_found")
}

// Execute answers the workspace's address for a connection still waiting for its provider.
func (h LocateConnectorReturn) Execute(ctx context.Context, state, kind string) (string, error) {
	awaited, known := h.Awaited[kind]
	tenant, named := shared.TenantOfReturnState(state)
	if !known || awaited == nil || !named {
		return "", connectorReturnNotFound()
	}
	// The hash each connector keeps its waiting connection under.
	hash := sha256.Sum256([]byte(state))
	var base string
	err := h.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: tenant},
		func(ctx context.Context) error {
			live, err := awaited.Live(ctx, hash[:], h.Clock.Now())
			if err != nil {
				return err
			}
			if !live {
				return connectorReturnNotFound()
			}
			base, err = h.Address.Of(ctx)
			return err
		})
	if errors.Is(err, shared.ErrNotFound) {
		// No such connection, or a workspace gone since it began: one answer.
		return "", connectorReturnNotFound()
	}
	if err != nil {
		return "", err
	}
	return base, nil
}

// Descriptor registers the lookup. Anonymous, as the provider's return is.
func (h LocateConnectorReturn) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: LocateConnectorReturnName,
		Summary: "Answers where a mailbox's, calendar push's or file store's provider return belongs: " +
			"given the state the provider carried back and the kind of connection, the address of " +
			"the workspace the connection belongs to, so the browser finishes there. Spends nothing. " +
			"A state no connection of that kind waits under is one refusal that names no workspace.",
		SideEffects: "None. Reads only.",
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "state", Kind: usecase.KindString, Required: true,
				Description: "The state the provider carried back."},
			{Name: "kind", Kind: usecase.KindString, Required: true,
				Description: "MAILBOX, CALENDAR_PUSH or FILE_STORE."},
		},
		Audit: usecase.AuditDeclaration{},
		Activity: usecase.ActivityDeclaration{
			Exempt: "Not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h LocateConnectorReturn) invoke(
	ctx context.Context, _ appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	base, err := h.Execute(ctx, in.String("state"), in.String("kind"))
	if err != nil {
		return nil, err
	}
	return usecase.Output{"base_url": base}, nil
}
