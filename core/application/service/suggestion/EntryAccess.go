// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// Subject is what an AI request about a target needs from reading it: the fingerprint the
// suggestion is judged stale against, and - for an entry - whose it is, which is what "assigned
// only" is measured against (domain-model.md §3.2).
type Subject struct {
	Digest   []byte
	Assignee shared.ID
}

// Subject reads the target through its ordinary read, as Digest does, and answers whose an entry
// is beside the fingerprint. The assignee is the stored one, never a caller's word: a caller that
// could name the assignee would be naming its own permission.
func (t EntryTargets) Subject(
	ctx context.Context, actor appshared.ActorContext,
	targetType domain.TargetType, targetID shared.ID,
) (Subject, error) {
	if targetType != domain.TargetWorkItem {
		digest, err := t.Digest(ctx, actor, targetType, targetID)
		return Subject{Digest: digest}, err
	}
	out, err := t.Catalogue.Invoke(ctx, readers[domain.TargetWorkItem], actor, usecase.Input{
		"item_id": targetID.String(),
	})
	if err != nil {
		return Subject{}, err
	}
	subject := Subject{Digest: domain.Digest(out.String("title"), out.String("notes"))}
	if assignee := out.String("assignee_id"); assignee != "" {
		if parsed, err := shared.ParseID(assignee); err == nil {
			subject.Assignee = parsed
		}
	}
	return subject, nil
}

// entryRequest is the permission question an AI request about one entry asks: along the entry's
// own path, so that a role held on its hub, its collection or the entry itself counts
// (domain-model.md §3.2) - the authoriser fills in the levels storage knows above the entry. `does`
// is what the request does to the entry: ItemChange for an ask, an acceptance or a dismissal, which
// is what changing the entry asks; ItemRead for the listing and the reading aids.
//
// The token scope is not asked here: the caller has asked it, because a reading aid admits either
// of two scopes and a request names one.
func entryRequest(
	permission service.Permission, does service.ItemAction, subject Subject, entryID shared.ID,
	action audit.Action, targetID shared.ID,
) access.Request {
	return access.Request{
		Permission: permission,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     action,
		TargetType: suggestionTarget,
		TargetID:   targetID,
		On:         access.ItemSubject{Does: does, ID: entryID, Assignee: subject.Assignee},
	}
}

// writesEntry is the question asking about an entry, accepting or dismissing a proposal about it
// asks: what changing it asks.
func writesEntry(subject Subject, entryID shared.ID, action audit.Action, targetID shared.ID) access.Request {
	return entryRequest(service.PermissionWriteItems, service.ItemChange, subject, entryID, action, targetID)
}

// readsEntry is the question the listing and the reading aids ask: what reading it asks.
func readsEntry(subject Subject, entryID shared.ID, action audit.Action, targetID shared.ID) access.Request {
	return entryRequest(service.PermissionRead, service.ItemRead, subject, entryID, action, targetID)
}
