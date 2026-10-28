// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// LocateOidcFlowName is the catalogue's name for the lookup.
const LocateOidcFlowName = "LocateOidcFlow"

// LocateOidcFlow answers where a provider's return belongs (multi-tenancy.md §3, identity.md §10).
//
// A provider returns to the installation's one registered address. In multi-tenant operation that
// is the installation's own host, while the flow began on a workspace's host - where the session
// that started a connection or a step-up lives, and the browser's marks of which return this is.
// So the return is finished there: this answers the workspace's main address - its main
// `tenant_host` row under the configured scheme and port (ADR-0095 §2) - for the workspace the state
// names, never from anything the request says, and spends nothing. Only a live flow is located; anything else is one refusal, which names no
// workspace.
type LocateOidcFlow struct {
	Flows      repository.OidcFlows
	Workspaces WorkspaceSlug
	// Hosts holds the workspace's main address. Nil, or a workspace without a serving main row,
	// answers the subdomain its short name makes.
	Hosts      repository.TenantHosts
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	// BaseURL and Multi are the installation's address and whether each workspace is its own
	// subdomain of it (domain.WorkspaceBase).
	BaseURL string
	Multi   bool
}

// WorkspaceSlug reads the workspace a transaction is bound to: its public name is what its address
// is made of.
type WorkspaceSlug interface {
	Find(ctx context.Context) (domain.Workspace, error)
}

// oidcFlowNotFound is the one answer for a state that names no live flow: unknown, expired, spent
// or malformed are not for a presenter to tell apart.
func oidcFlowNotFound() error {
	return shared.ErrNotFound.WithDetail("auth.oidc_flow_not_found")
}

// Execute answers the workspace's address for a live flow.
func (h LocateOidcFlow) Execute(ctx context.Context, presented string) (string, error) {
	state, err := domain.ParseOidcFlowState(presented)
	if err != nil {
		return "", oidcFlowNotFound()
	}
	var base string
	err = h.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: state.TenantID()},
		func(ctx context.Context) error {
			live, err := h.Flows.Live(ctx, state, h.Clock.Now())
			if err != nil {
				return err
			}
			if !live {
				return oidcFlowNotFound()
			}
			if !h.Multi {
				base = domain.WorkspaceBase(h.BaseURL, "", false)
				return nil
			}
			if h.Hosts != nil {
				hosts, err := h.Hosts.List(ctx)
				if err != nil {
					return err
				}
				if main := domain.MainHostOf(hosts); main != "" {
					base = domain.WorkspaceBaseAt(h.BaseURL, main)
					return nil
				}
			}
			workspace, err := h.Workspaces.Find(ctx)
			if err != nil {
				return err
			}
			base = domain.WorkspaceBase(h.BaseURL, workspace.Slug, true)
			return nil
		})
	if errors.Is(err, shared.ErrNotFound) {
		// The flow's absence, or a workspace gone between the start and the return: one answer.
		return "", oidcFlowNotFound()
	}
	if err != nil {
		return "", err
	}
	return base, nil
}

// Descriptor registers the lookup. Anonymous, as the provider's return is.
func (h LocateOidcFlow) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: LocateOidcFlowName,
		Summary: "Answers where a provider's return belongs: given the state the provider carried " +
			"back, the address of the workspace the flow belongs to, so the browser finishes there. " +
			"Spends nothing. A state that names no live flow - unknown, expired, spent or malformed - " +
			"is one refusal that names no workspace.",
		SideEffects: "None. Reads only.",
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "state", Kind: usecase.KindString, Required: true,
				Description: "The handle the start answered, as the provider carried it back."},
		},
		Audit: usecase.AuditDeclaration{},
		Activity: usecase.ActivityDeclaration{
			Exempt: "Not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h LocateOidcFlow) invoke(
	ctx context.Context, _ appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	base, err := h.Execute(ctx, in.String("state"))
	if err != nil {
		return nil, err
	}
	return usecase.Output{"base_url": base}, nil
}
