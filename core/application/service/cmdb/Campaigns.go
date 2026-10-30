// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"context"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The use case names (UC-AST-10).
const (
	ListCiCampaignsName = "ListCiCampaigns"
	StartCiCampaignName = "StartCiCampaign"
	ReadCiCampaignName  = "ReadCiCampaign"
	AttestCiName        = "AttestCi"
	CloseCiCampaignName = "CloseCiCampaign"
)

// The audit codes of certification. An attestation names the CI and the verdict, never a name.
const (
	CampaignStartedAction audit.Action = "cmdb.campaign_started"
	CampaignClosedAction  audit.Action = "cmdb.campaign_closed"
	CIAttestedAction      audit.Action = "cmdb.ci_attested"
)

const campaignTarget = "cmdb_campaign"

func dayOf(in usecase.Input, field string) (*time.Time, error) {
	raw := in.String(field)
	if raw == "" {
		return nil, nil
	}
	d, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		return nil, invalid("/"+field, "cmdb.day_malformed")
	}
	return &d, nil
}

func dayOrNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Format(time.DateOnly)
}

// CampaignOutput is a campaign with its score (schema CiCampaign).
func CampaignOutput(c domain.Campaign, s domain.Score) usecase.Output {
	var closedAt any
	if c.ClosedAt != nil {
		closedAt = *c.ClosedAt
	}
	return usecase.Output{
		"id": c.ID.String(), "hub_id": c.HubID.String(), "name": c.Name, "classes": append([]string{}, c.Classes...),
		"due_by": dayOrNil(c.DueBy), "due_on": c.DueOn.Format(time.DateOnly), "review_months": c.ReviewMonths,
		"started_by": c.StartedBy.String(), "started_at": c.StartedAt, "closed_at": closedAt,
		"version": c.Version, "score": usecase.Output{
			"total": s.Total, "confirmed": s.Confirmed, "disputed": s.Disputed, "pending": s.Pending,
			"overdue": s.Overdue, "rate": s.Rate,
		},
	}
}

func scoreOf(c domain.Campaign, counts map[domain.Verdict]int, now time.Time) domain.Score {
	var as []domain.Attestation
	for v, n := range counts {
		for range n {
			as = append(as, domain.Attestation{Verdict: v})
		}
	}
	return c.Tally(as, now)
}

// ListCiCampaigns answers a hub's certification campaigns, the latest first.
type ListCiCampaigns struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h ListCiCampaigns) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListCiCampaignsName, Permission: service.PermissionRead,
		Summary: "Lists a hub's certification campaigns, the latest first, each with its score: how many CIs were " +
			"asked, confirmed, disputed, pending and overdue, and the share answered.",
		SideEffects: "None. Reads only.", TokenScope: cisRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub."},
			{Name: "limit", Kind: usecase.KindInt, Description: "At most this many, 1 to 100; 50 when not given."},
		},
		Audit: usecase.AuditDeclaration{Action: CIReadAction, TargetType: campaignTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			s := h.Inventory
			if err := s.authorizeHub(ctx, actor, hub, false, CIReadAction, campaignTarget, hub); err != nil {
				return nil, err
			}
			data := []usecase.Output{}
			err = s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := s.hub(ctx, hub); err != nil {
					return err
				}
				cs, err := s.Assurance.Campaigns(ctx, hub, limitOf(in, 50, 100))
				if err != nil {
					return err
				}
				ids := make([]shared.ID, 0, len(cs))
				for _, c := range cs {
					ids = append(ids, c.ID)
				}
				tallies, err := s.Assurance.Tallies(ctx, ids)
				if err != nil {
					return err
				}
				for _, c := range cs {
					data = append(data, CampaignOutput(c, scoreOf(c, tallies[c.ID], s.Clock.Now())))
				}
				return nil
			})
			return usecase.Output{"data": data}, err
		}),
	})
}

// StartCiCampaign asks the owners of a hub's CIs to attest them.
type StartCiCampaign struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h StartCiCampaign) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: StartCiCampaignName, Permission: service.PermissionWriteItems,
		Summary: "Starts a certification campaign in a hub: every active CI of the classes named (all when none) " +
			"- and, with due_by, only those whose review falls due by then or was never set - is asked once, of " +
			"its owning person when it has one. Answers are due on due_on; a confirmed CI's next review moves " +
			"review_months on (12 when not given). The set is fixed when it starts.",
		SideEffects: "Writes the campaign, one pending attestation per CI asked and an audit entry.", TokenScope: cisWrite,
		Input: []usecase.Field{
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub."},
			{Name: "name", Kind: usecase.KindString, Required: true, Description: "What it is called."},
			{Name: "classes", Kind: usecase.KindList, Description: "The classes asked, with those below them."},
			{Name: "due_by", Kind: usecase.KindString, Description: "Only CIs whose review falls due by this day, YYYY-MM-DD."},
			{Name: "due_on", Kind: usecase.KindString, Required: true, Description: "The day answers are due, YYYY-MM-DD."},
			{Name: "review_months", Kind: usecase.KindInt, Description: "How far a confirmed CI's review moves, 1 to 60."},
		},
		Audit: usecase.AuditDeclaration{Action: CampaignStartedAction, TargetType: campaignTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			s := h.Inventory
			if err := s.authorizeHub(ctx, actor, hub, true, CampaignStartedAction, campaignTarget, hub); err != nil {
				return nil, err
			}
			dueBy, err := dayOf(in, "due_by")
			if err != nil {
				return nil, err
			}
			dueOn, err := dayOf(in, "due_on")
			if err != nil {
				return nil, err
			}
			if dueOn == nil {
				return nil, invalid("/due_on", "cmdb.day_malformed")
			}
			classes, err := in.StringList("classes")
			if err != nil {
				return nil, err
			}
			var out usecase.Output
			err = s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := s.hub(ctx, hub); err != nil {
					return err
				}
				reg, err := s.registry(ctx)
				if err != nil {
					return err
				}
				now := s.Clock.Now()
				c, err := domain.NewCampaign(s.IDs.NewID(), actor.TenantID, hub, actor.AccountID, domain.CampaignInput{
					Name: in.String("name"), Classes: classes, DueBy: dueBy, DueOn: *dueOn,
					ReviewMonths: in.Int("review_months"),
				}, reg.classes, now)
				if err != nil {
					return err
				}
				cis, err := s.hubCIs(ctx, hub)
				if err != nil {
					return err
				}
				var asked []domain.Attestation
				for _, ci := range cis {
					if c.InScope(ci, reg.classes) {
						asked = append(asked, domain.Attestation{CampaignID: c.ID, CIID: ci.ID, Owner: ci.Compliance.OwnerAccount, Verdict: domain.VerdictPending})
					}
				}
				if len(asked) == 0 {
					return invalid("/classes", "cmdb.campaign_asks_nothing")
				}
				if len(asked) > domain.MaxCampaignCIs {
					return invalid("/classes", "cmdb.campaign_too_large")
				}
				if err := s.Assurance.InsertCampaign(ctx, c, asked); err != nil {
					return err
				}
				out = CampaignOutput(c, c.Tally(asked, now))
				return s.record(ctx, actor, CampaignStartedAction, campaignTarget, c.ID,
					audit.Change{Field: "name", Classification: audit.Sensitive, To: c.Name},
					audit.Change{Field: "asked", Classification: audit.Open, To: itoa(len(asked))})
			})
			return out, err
		}),
	})
}

// ReadCiCampaign answers a campaign with every CI asked and its answer.
type ReadCiCampaign struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h ReadCiCampaign) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadCiCampaignName, Permission: service.PermissionRead,
		Summary: "Reads a certification campaign with its score and every CI asked: its name and class, the person " +
			"asked, the verdict, who gave it and when.",
		SideEffects: "None. Reads only.", TokenScope: cisRead, ReadOnly: true,
		Input: []usecase.Field{{Name: "campaign_id", Kind: usecase.KindID, Required: true, Description: "The campaign."}},
		Audit: usecase.AuditDeclaration{Action: CIReadAction, TargetType: campaignTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("campaign_id")
			if err != nil {
				return nil, err
			}
			s := h.Inventory
			c, err := s.campaign(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			if err := s.authorizeHub(ctx, actor, c.HubID, false, CIReadAction, campaignTarget, id); err != nil {
				return nil, err
			}
			var out usecase.Output
			err = s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				as, err := s.Assurance.Attestations(ctx, id)
				if err != nil {
					return err
				}
				ids := make([]shared.ID, 0, len(as))
				for _, a := range as {
					ids = append(ids, a.CIID)
				}
				cis, err := s.Store.CIsByID(ctx, ids)
				if err != nil {
					return err
				}
				byID := map[shared.ID]domain.CI{}
				for _, ci := range cis {
					byID[ci.ID] = ci
				}
				rows := make([]usecase.Output, 0, len(as))
				for _, a := range as {
					ci := byID[a.CIID]
					var at any
					if a.AttestedAt != nil {
						at = *a.AttestedAt
					}
					rows = append(rows, usecase.Output{
						"ci_id": a.CIID.String(), "name": ci.Name, "class": ci.Class, "owner_id": idOrNil(a.Owner),
						"verdict": string(a.Verdict), "attested_by": idOrNil(a.AttestedBy), "attested_at": at,
					})
				}
				out = CampaignOutput(c, c.Tally(as, s.Clock.Now()))
				out["attestations"] = rows
				return nil
			})
			return out, err
		}),
	})
}

func (s Inventory) campaign(ctx context.Context, actor appshared.ActorContext, id shared.ID) (domain.Campaign, error) {
	var c domain.Campaign
	err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		c, err = s.Assurance.FindCampaign(ctx, id)
		return err
	})
	return c, err
}

// AttestCi answers a campaign's question about one CI.
type AttestCi struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h AttestCi) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: AttestCiName, Permission: service.PermissionRead,
		Summary: "Answers a campaign's question about a CI: CONFIRMED - it is as recorded, and its next review " +
			"moves the campaign's period on -, or DISPUTED - the record is wrong or the object gone, and its review " +
			"falls due today. The person asked answers with READ on the hub; anybody else needs WRITE_ITEMS. A " +
			"closed campaign takes no answer; a later answer replaces an earlier one.",
		SideEffects: "Writes the attestation, the CI's review day, an audit entry and the change log.", TokenScope: cisWrite,
		Input: []usecase.Field{
			{Name: "campaign_id", Kind: usecase.KindID, Required: true, Description: "The campaign."},
			{Name: "ci_id", Kind: usecase.KindID, Required: true, Description: "The configuration item."},
			{Name: "verdict", Kind: usecase.KindString, Required: true, Enum: []string{"CONFIRMED", "DISPUTED"}, Description: "The answer."},
		},
		Audit: usecase.AuditDeclaration{Action: CIAttestedAction, TargetType: ciTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("campaign_id")
			if err != nil {
				return nil, err
			}
			ciID, err := in.ID("ci_id")
			if err != nil {
				return nil, err
			}
			s := h.Inventory
			c, err := s.campaign(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			var asked domain.Attestation
			err = s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				as, err := s.Assurance.Attestations(ctx, id)
				for _, a := range as {
					if a.CIID == ciID {
						asked = a
					}
				}
				return err
			})
			if err != nil {
				return nil, err
			}
			owner := !asked.Owner.IsZero() && asked.Owner == actor.AccountID
			if err := s.authorizeHub(ctx, actor, c.HubID, !owner, CIAttestedAction, ciTarget, ciID); err != nil {
				return nil, err
			}
			var out usecase.Output
			err = s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				a, err := s.Assurance.FindAttestation(ctx, id, ciID)
				if err != nil {
					return err
				}
				now := s.Clock.Now()
				a, err = c.Answer(a, domain.Verdict(in.String("verdict")), actor.AccountID, now)
				if err != nil {
					return err
				}
				if err := s.Assurance.Answer(ctx, a); err != nil {
					return err
				}
				ci, err := s.Store.FindCI(ctx, ciID, true)
				if err != nil {
					return err
				}
				next := c.NextReview(now)
				if a.Verdict == domain.VerdictDisputed {
					y, m, d := now.UTC().Date()
					next = time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
				}
				updated := ci
				updated.Compliance.ReviewDue = &next
				updated.UpdatedBy, updated.UpdatedAt = actor.AccountID, now
				if err := s.Store.UpdateCI(ctx, updated, ci.Version); err != nil {
					return err
				}
				updated.Version++
				if err := s.announce(ctx, actor, updated); err != nil {
					return err
				}
				out = usecase.Output{
					"campaign_id": id.String(), "ci_id": ciID.String(), "verdict": string(a.Verdict),
					"attested_by": actor.AccountID.String(), "attested_at": now, "review_due": next.Format(time.DateOnly),
				}
				return s.record(ctx, actor, CIAttestedAction, ciTarget, ciID,
					audit.Change{Field: "campaign_id", Classification: audit.Open, To: id.String()},
					audit.Change{Field: "verdict", Classification: audit.Open, To: string(a.Verdict)})
			})
			return out, err
		}),
	})
}

// CloseCiCampaign stops a campaign taking answers; its score stays as it stood.
type CloseCiCampaign struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h CloseCiCampaign) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: CloseCiCampaignName, Permission: service.PermissionWriteItems,
		Summary: "Closes a certification campaign: it takes no more answers and its score stays as it stood, the " +
			"unanswered ones counted as pending.",
		SideEffects: "Writes the campaign and an audit entry.", TokenScope: cisWrite,
		Input: []usecase.Field{
			{Name: "campaign_id", Kind: usecase.KindID, Required: true, Description: "The campaign."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read."},
		},
		Audit: usecase.AuditDeclaration{Action: CampaignClosedAction, TargetType: campaignTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("campaign_id")
			if err != nil {
				return nil, err
			}
			s := h.Inventory
			c, err := s.campaign(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			if err := s.authorizeHub(ctx, actor, c.HubID, true, CampaignClosedAction, campaignTarget, id); err != nil {
				return nil, err
			}
			if c.Closed() {
				return nil, shared.ErrConflict.WithDetail("cmdb.campaign_closed")
			}
			expected := c.Version
			if in.Present("expected_version") {
				expected = in.Int("expected_version")
			}
			var out usecase.Output
			err = s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				now := s.Clock.Now()
				if err := s.Assurance.CloseCampaign(ctx, id, now, actor.AccountID, expected); err != nil {
					return err
				}
				tallies, err := s.Assurance.Tallies(ctx, []shared.ID{id})
				if err != nil {
					return err
				}
				c.ClosedAt, c.ClosedBy, c.Version = &now, actor.AccountID, c.Version+1
				out = CampaignOutput(c, scoreOf(c, tallies[id], now))
				return s.record(ctx, actor, CampaignClosedAction, campaignTarget, id)
			})
			return out, err
		}),
	})
}
