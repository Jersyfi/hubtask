// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"context"
	"errors"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// pass is one reconciliation in its transaction: the registry, the run it writes as, and what
// the items so far became.
type pass struct {
	s       Inventory
	actor   appshared.ActorContext
	hubID   shared.ID
	reg     registry
	source  domain.Source
	run     domain.Run
	dry     bool
	refs    map[string]shared.ID
	holds   []domain.Hold
	count   int
	written map[shared.ID]bool
}

func (s Inventory) newPass(ctx context.Context, actor appshared.ActorContext, hubID shared.ID, sourceKey string, dry bool) (*pass, error) {
	if err := s.hub(ctx, hubID); err != nil {
		return nil, err
	}
	reg, err := s.registry(ctx)
	if err != nil {
		return nil, err
	}
	source, ok := reg.sources[sourceKey]
	if !ok {
		return nil, shared.ErrNotFound.WithDetail("cmdb.source_not_found").WithParams(map[string]string{"key": sourceKey})
	}
	count, err := s.Store.CountCIs(ctx, hubID)
	if err != nil {
		return nil, err
	}
	return &pass{
		s: s, actor: actor, hubID: hubID, reg: reg, source: source, dry: dry, count: count,
		refs: map[string]shared.ID{}, written: map[shared.ID]bool{},
		run:  domain.Run{ID: s.IDs.NewID(), HubID: hubID, Source: sourceKey, ActorID: actor.AccountID, StartedAt: s.Clock.Now()},
	}, nil
}

// codeOf is a refusal's detail code, for an explanation that names no value.
func codeOf(err error) (string, bool) {
	var e *shared.Error
	if errors.As(err, &e) && e.Category == shared.CategoryValidation {
		if e.DetailCode != "" {
			return e.DetailCode, true
		}
		return e.Code, true
	}
	return "", false
}

// item reconciles one item: identify, then create, update, hold or open a duplicate (R3-R7).
func (p *pass) item(ctx context.Context, item domain.Item) (domain.Decision, error) {
	d := domain.Decision{Ref: item.Ref, Class: item.Class}
	class, ok := p.reg.classes.Find(item.Class)
	if !ok || class.Abstract || class.ArchivedAt != nil {
		d.Outcome, d.Reason = domain.OutcomeRefused, "cmdb.class_not_instantiable"
		return d, nil
	}
	if !p.source.Writes(item.Class, p.reg.classes) {
		return p.hold(item, d, domain.HoldClassForbidden), nil
	}
	resolved, ident, err := p.identify(ctx, item)
	if err != nil {
		return d, err
	}
	d.Rule = ident.Rule
	switch {
	case len(ident.Matches) > 1:
		d.Outcome, d.Candidates = domain.OutcomeDuplicate, ident.Matches
		if !p.dry && len(ident.Matches) <= 50 {
			if _, err := p.s.Store.OpenDuplicate(ctx, domain.Duplicate{
				ID: p.s.IDs.NewID(), HubID: p.hubID, CIs: ident.Matches, Rule: ident.Rule, CreatedAt: p.run.StartedAt,
			}); err != nil {
				return d, err
			}
		}
		return d, nil
	case len(ident.Matches) == 1:
		ci, err := p.s.Store.FindCI(ctx, ident.Matches[0], true)
		if err != nil {
			return d, err
		}
		if p.written[ci.ID] {
			d.Outcome, d.CI = domain.OutcomeUnchanged, ci.ID
			p.refs[item.Ref] = ci.ID
			return d, nil
		}
		return p.update(ctx, ci, item, d)
	}
	if !ident.Usable && p.source.Kind != domain.SourceManual {
		return p.hold(item, d, domain.HoldInsufficient), nil
	}
	if !p.source.MayCreate {
		return p.hold(item, d, domain.HoldMayNotCreate), nil
	}
	return p.create(ctx, item, resolved, d)
}

// identify looks an item up in the hub's identifier index.
func (p *pass) identify(ctx context.Context, item domain.Item) (domain.Resolved, domain.Identification, error) {
	resolved := p.reg.classes.Resolve(item.Class)
	ids := domain.ItemIdentifiers(item, resolved, p.source.Key)
	rows, err := p.s.Store.FindIdentifiers(ctx, p.hubID, ids)
	if err != nil {
		return resolved, domain.Identification{}, err
	}
	return resolved, domain.Identify(resolved, ids, p.source.Key, rows, p.reg.classes), nil
}

func (p *pass) hold(item domain.Item, d domain.Decision, reason string) domain.Decision {
	d.Outcome, d.Reason = domain.OutcomeHold, reason
	p.holds = append(p.holds, domain.Hold{
		ID: p.s.IDs.NewID(), HubID: p.hubID, Run: p.run.ID, Source: p.source.Key, Item: item, Reason: reason,
		CreatedAt: p.run.StartedAt,
	})
	return d
}

func (p *pass) create(ctx context.Context, item domain.Item, class domain.Resolved, d domain.Decision) (domain.Decision, error) {
	if p.count >= domain.MaxCIsPerHub {
		d.Outcome, d.Reason = domain.OutcomeRefused, "cmdb.hub_full"
		return d, nil
	}
	now := p.run.StartedAt
	ci, taken, kept, err := domain.Apply(domain.CI{}, item, class, p.source.Key, p.run.ID, p.reg.ranks, now)
	if err != nil {
		if code, ok := codeOf(err); ok {
			d.Outcome, d.Reason = domain.OutcomeRefused, code
			return d, nil
		}
		return d, err
	}
	ci.ID, ci.TenantID, ci.HubID, ci.Class = p.s.IDs.NewID(), p.actor.TenantID, p.hubID, item.Class
	ci.Stage, ci.Status, ci.Compliance.Quantity = domain.StageInUse, domain.StatusOK, 1
	ci.CreatedBy, ci.CreatedAt, ci.UpdatedBy, ci.UpdatedAt, ci.Version = p.actor.AccountID, now, p.actor.AccountID, now, 1
	d.Outcome, d.CI, d.Taken, d.Kept = domain.OutcomeCreate, ci.ID, taken, kept
	p.refs[item.Ref] = ci.ID
	p.count++
	if p.dry {
		return d, nil
	}
	if err := p.s.Store.InsertCI(ctx, ci); err != nil {
		return d, err
	}
	p.written[ci.ID] = true
	if err := p.s.Store.PutIdentifiers(ctx, ci, p.identifiers(ci, class, item)); err != nil {
		return d, err
	}
	if err := p.s.announce(ctx, p.actor, ci); err != nil {
		return d, err
	}
	return d, p.s.recordCI(ctx, p.actor, CICreatedAction, ci, append([]string{domain.FieldClass}, taken...))
}

func (p *pass) identifiers(ci domain.CI, class domain.Resolved, item domain.Item) map[string]string {
	ids := ci.Identifiers(class)
	native := domain.NativeKey(p.source.Key)
	if key, ok := domain.ItemIdentifiers(item, class, p.source.Key)[native]; ok {
		ids[native] = key
	}
	return ids
}

func (p *pass) update(ctx context.Context, ci domain.CI, item domain.Item, d domain.Decision) (domain.Decision, error) {
	// An item of a more specific class refines the CI; one of another branch keeps the CI's.
	class := ci.Class
	if item.Class != ci.Class && p.reg.classes.IsA(item.Class, ci.Class) {
		class = item.Class
	}
	resolved := p.reg.classes.Resolve(class)
	next, taken, kept, err := domain.Apply(ci, item, resolved, p.source.Key, p.run.ID, p.reg.ranks, p.run.StartedAt)
	if err != nil {
		if code, ok := codeOf(err); ok {
			d.Outcome, d.Reason, d.CI = domain.OutcomeRefused, code, ci.ID
			return d, nil
		}
		return d, err
	}
	if class != ci.Class {
		taken = append(taken, domain.FieldClass)
	}
	next.Class, next.UpdatedBy, next.UpdatedAt = class, p.actor.AccountID, p.run.StartedAt
	d.CI, d.Class, d.Taken, d.Kept = ci.ID, class, taken, kept
	d.Outcome = domain.OutcomeUpdate
	if len(taken) == 0 {
		d.Outcome = domain.OutcomeUnchanged
	}
	p.refs[item.Ref] = ci.ID
	if p.dry {
		return d, nil
	}
	if err := p.s.Store.UpdateCI(ctx, next, ci.Version); err != nil {
		return d, err
	}
	p.written[ci.ID] = true
	if err := p.s.Store.PutIdentifiers(ctx, next, p.identifiers(next, resolved, item)); err != nil {
		return d, err
	}
	if len(taken) == 0 {
		// Only the sighting moved; a device's copy keeps the last sighting it was told of.
		return d, nil
	}
	next.Version = ci.Version + 1
	if err := p.s.announce(ctx, p.actor, next); err != nil {
		return d, err
	}
	return d, p.s.recordCI(ctx, p.actor, CIChangedAction, next, taken)
}

// relation writes one reported relation, or sees it again.
func (p *pass) relation(ctx context.Context, r domain.RelationItem) error {
	from, to := r.FromID, r.ToID
	if r.From != "" {
		from = p.refs[r.From]
	}
	if r.To != "" {
		to = p.refs[r.To]
	}
	t, ok := p.reg.types.Find(r.Type)
	if from.IsZero() || to.IsZero() || !ok {
		p.run.Counts.RelationsRefused++
		return nil
	}
	cis, err := p.s.Store.CIsByID(ctx, []shared.ID{from, to})
	if err != nil {
		return err
	}
	if len(cis) != 2 {
		// A CI only this dry run would have made: judged by its item's class.
		p.run.Counts.RelationsAdded++
		return nil
	}
	source, target := cis[0], cis[1]
	if source.ID != from {
		source, target = target, source
	}
	if domain.CheckRelation(t, source, target, p.reg.classes) != nil {
		p.run.Counts.RelationsRefused++
		return nil
	}
	if p.dry {
		p.run.Counts.RelationsSeen++
		return nil
	}
	rel := domain.Relation{
		ID: p.s.IDs.NewID(), HubID: p.hubID, Type: t.Key, SourceCI: from, TargetCI: to, DataSource: p.source.Key,
		FirstSeen: p.run.StartedAt, LastSeen: p.run.StartedAt, CreatedBy: p.actor.AccountID,
	}
	stored, inserted, err := p.s.Store.PutRelation(ctx, rel)
	if err != nil {
		return err
	}
	if !inserted {
		p.run.Counts.RelationsSeen++
		return nil
	}
	p.run.Counts.RelationsAdded++
	rel.ID = stored
	return p.s.announceRelations(ctx, p.actor, rel)
}

// reconcile runs a payload through the one write path inside the caller's transaction and
// answers the run. A dry run writes nothing and records nothing (R9).
func (s Inventory) reconcile(ctx context.Context, actor appshared.ActorContext, hubID shared.ID, sourceKey string, payload domain.Payload, dry bool) (domain.Run, error) {
	if len(payload.Items) > domain.MaxPayloadItems {
		return domain.Run{}, invalid("/items", "cmdb.payload_too_large", "maximum", itoa(domain.MaxPayloadItems))
	}
	if len(payload.Relations) > domain.MaxPayloadRelations {
		return domain.Run{}, invalid("/relations", "cmdb.payload_too_large", "maximum", itoa(domain.MaxPayloadRelations))
	}
	p, err := s.newPass(ctx, actor, hubID, sourceKey, dry)
	if err != nil {
		return domain.Run{}, err
	}
	for i, item := range payload.Items {
		if item.Ref == "" {
			item.Ref = "item-" + itoa(i+1)
		}
		d, err := p.item(ctx, item)
		if err != nil {
			return domain.Run{}, err
		}
		p.run.Counts.Count(d)
		p.run.Decisions = append(p.run.Decisions, d)
	}
	for _, r := range payload.Relations {
		if err := p.relation(ctx, r); err != nil {
			return domain.Run{}, err
		}
	}
	if dry {
		return p.run, nil
	}
	if err := s.Store.InsertRun(ctx, p.run); err != nil {
		return domain.Run{}, err
	}
	for _, h := range p.holds {
		if err := s.Store.InsertHold(ctx, h); err != nil {
			return domain.Run{}, err
		}
	}
	if p.source.Kind != domain.SourceManual {
		if err := s.Store.TouchSource(ctx, sourceKey, p.run.StartedAt); err != nil {
			return domain.Run{}, err
		}
	}
	return p.run, nil
}

func invalid(path, code string, params ...string) error {
	return fileInvalid(path, code, params...)
}
