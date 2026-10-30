// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"strings"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// A portal's organisations (UC-SVC-11): requesters grouped by the mail domain of their address,
// the workspace's configuration - written with STRUCTURE at the workspace, read with READ there.
const (
	ListRequesterOrganisationsName  = "ListRequesterOrganisations"
	DefineRequesterOrganisationName = "DefineRequesterOrganisation"
	DeleteRequesterOrganisationName = "DeleteRequesterOrganisation"

	organisationTarget = "requester_organisation"

	OrganisationDefinedAction audit.Action = "requester_organisation.defined"
	OrganisationDeletedAction audit.Action = "requester_organisation.deleted"
	OrganisationsReadAction   audit.Action = "requester_organisation.read"
)

// OrganisationOutput is the shape every door returns (api/openapi.yaml, RequesterOrganisation).
func OrganisationOutput(o domain.Organisation) usecase.Output {
	domains := make([]any, 0, len(o.Domains))
	for _, d := range o.Domains {
		domains = append(domains, d)
	}
	return usecase.Output{
		"key": o.Key, "name": o.Name, "domains": domains, "shares_requests": o.SharesRequests, "version": o.Version,
	}
}

// ListRequesterOrganisations reads the workspace's organisations.
type ListRequesterOrganisations struct{ Desk Desk }

// Execute answers them by key.
func (h ListRequesterOrganisations) Execute(ctx context.Context, actor appshared.ActorContext) ([]domain.Organisation, error) {
	if err := h.Desk.authorizeRead(ctx, actor, OrganisationsReadAction, organisationTarget); err != nil {
		return nil, err
	}
	var out []domain.Organisation
	err := h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		out, err = h.Desk.Organisations.List(ctx)
		return err
	})
	return out, err
}

// Descriptor is the catalogue entry.
func (h ListRequesterOrganisations) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListRequesterOrganisationsName, Permission: service.PermissionRead,
		Summary: "Lists the portal's organisations: each one's key, name, the mail domains its people are " +
			"assigned by, and whether they see each other's requests.",
		SideEffects: "None. Reads only.", TokenScope: deskRead, ReadOnly: true,
		Audit: usecase.AuditDeclaration{Action: OrganisationsReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			organisations, err := h.Execute(ctx, actor)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(organisations))
			for _, o := range organisations {
				data = append(data, OrganisationOutput(o))
			}
			return usecase.Output{"data": data}, nil
		}),
	}, organisationTarget)
}

// DefineRequesterOrganisationCommand is the input, typed.
type DefineRequesterOrganisationCommand struct {
	Key, Name       string
	Domains         []string
	SharesRequests  bool
	ExpectedVersion int
}

// DefineRequesterOrganisation creates an organisation under a key, or replaces the one there.
type DefineRequesterOrganisation struct{ Desk Desk }

// Execute defines it; a domain another organisation holds is `organisations.domain_taken`.
func (h DefineRequesterOrganisation) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd DefineRequesterOrganisationCommand,
) (domain.Organisation, error) {
	if err := h.Desk.authorizeWrite(ctx, actor, OrganisationDefinedAction, organisationTarget); err != nil {
		return domain.Organisation{}, err
	}
	now := h.Desk.Clock.Now()
	var out domain.Organisation
	err := h.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		current, err := h.Desk.Organisations.Find(ctx, strings.TrimSpace(cmd.Key))
		switch {
		case errors.Is(err, shared.ErrNotFound):
			held, err := h.Desk.Organisations.Count(ctx)
			if err != nil {
				return err
			}
			if held >= domain.MaxOrganisations {
				return shared.ErrConflict.WithDetail("organisations.too_many").
					WithParams(map[string]string{"max": "500"})
			}
			if out, err = domain.NewOrganisation(domain.NewOrganisationInput{
				ID: h.Desk.IDs.NewID(), TenantID: actor.TenantID, Key: cmd.Key, Name: cmd.Name,
				Domains: cmd.Domains, SharesRequests: cmd.SharesRequests, Now: now, Text: h.Desk.Text,
			}); err != nil {
				return err
			}
			if err := h.Desk.Organisations.Insert(ctx, out); err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			if cmd.ExpectedVersion != 0 && cmd.ExpectedVersion != current.Version {
				return shared.ErrVersionConflict.WithDetail("organisations.version_conflict")
			}
			if out, err = current.Change(cmd.Name, cmd.Domains, cmd.SharesRequests, now, h.Desk.Text); err != nil {
				return err
			}
			if err := h.Desk.Organisations.Update(ctx, out, current.Version); err != nil {
				return err
			}
			out.Version = current.Version + 1
		}
		// The name is a person's words (rule 10); the domains are the workspace's configuration.
		return h.Desk.record(ctx, actor, OrganisationDefinedAction, organisationTarget, out.ID, now,
			audit.Change{Field: "key", Classification: audit.Open, To: out.Key},
			audit.Change{Field: "name", Classification: audit.Sensitive, To: out.Name},
			audit.Change{Field: "domains", Classification: audit.Open, To: strings.Join(out.Domains, ",")},
			audit.Change{Field: "shares_requests", Classification: audit.Open, To: boolText(out.SharesRequests)})
	})
	return out, err
}

func boolText(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// Descriptor is the catalogue entry.
func (h DefineRequesterOrganisation) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: DefineRequesterOrganisationName,
		Summary: "Creates a portal organisation under a key, or replaces the one there: every person whose " +
			"address is at one of its mail domains belongs to it, and with shares_requests they see each " +
			"other's requests in the portal. A domain is in at most one organisation.",
		SideEffects: "Writes the organisation and an audit entry.",
		TokenScope:  deskWrite,
		Input: []usecase.Field{
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "Small letters, digits and underscores, starting with a letter."},
			{Name: "name", Kind: usecase.KindString, Required: true, Description: "Up to 120 characters, on one line."},
			{Name: "domains", Kind: usecase.KindList, Required: true, Description: "One to twenty mail domains such as example.org."},
			{Name: "shares_requests", Kind: usecase.KindBool, Description: "Whether its people see each other's requests; off when left out."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read; a replacement of a newer one is refused."},
		},
		Audit: usecase.AuditDeclaration{Action: OrganisationDefinedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			domains, err := stringList(in, "domains")
			if err != nil {
				return nil, err
			}
			defined, err := h.Execute(ctx, actor, DefineRequesterOrganisationCommand{
				Key: in.String("key"), Name: in.String("name"), Domains: domains,
				SharesRequests: in.Bool("shares_requests"), ExpectedVersion: in.Int("expected_version"),
			})
			if err != nil {
				return nil, err
			}
			return OrganisationOutput(defined), nil
		}),
	}, organisationTarget)
}

// stringList reads a list of texts; anything else in it is refused.
func stringList(in usecase.Input, name string) ([]string, error) {
	raw, _ := in[name].([]any)
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		s, ok := v.(string)
		if !ok {
			return nil, malformed("/"+name, "organisations.domain_malformed")
		}
		out = append(out, s)
	}
	return out, nil
}

// DeleteRequesterOrganisation removes an organisation; its people see their own requests only.
type DeleteRequesterOrganisation struct{ Desk Desk }

// Execute deletes it.
func (h DeleteRequesterOrganisation) Execute(ctx context.Context, actor appshared.ActorContext, key string) error {
	if err := h.Desk.authorizeWrite(ctx, actor, OrganisationDeletedAction, organisationTarget); err != nil {
		return err
	}
	now := h.Desk.Clock.Now()
	return h.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		current, err := h.Desk.Organisations.Find(ctx, key)
		if err != nil {
			return err
		}
		if err := h.Desk.Organisations.Delete(ctx, key); err != nil {
			return err
		}
		return h.Desk.record(ctx, actor, OrganisationDeletedAction, organisationTarget, current.ID, now,
			audit.Change{Field: "key", Classification: audit.Open, From: current.Key})
	})
}

// Descriptor is the catalogue entry.
func (h DeleteRequesterOrganisation) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name:        DeleteRequesterOrganisationName,
		Summary:     "Deletes a portal organisation. Its people keep their own requests and see only those again.",
		SideEffects: "Deletes the organisation and writes an audit entry.",
		TokenScope:  deskWrite,
		Input: []usecase.Field{
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "The organisation's key."},
		},
		Audit: usecase.AuditDeclaration{Action: OrganisationDeletedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			if err := h.Execute(ctx, actor, in.String("key")); err != nil {
				return nil, err
			}
			return usecase.Output{}, nil
		}),
	}, organisationTarget)
}
