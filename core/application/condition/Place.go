// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package condition

import (
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// PlaceOf is where an event happened as the authoriser reads a path: the workspace, then the hub,
// collection or entry the event names. The authoriser completes a collection with its hub and
// reads an entry's hub from storage, so the payload's own identifiers are enough to ask whether
// somebody reaches the place - a private hub's events are kept from whoever does not
// (ADR-0073 §1). An event about nothing in a hub answers the workspace alone.
func PlaceOf(envelope event.Envelope) []identity.Scope {
	path := []identity.Scope{identity.TenantScope()}
	if hub := HubOf(envelope); !hub.IsZero() {
		return append(path, identity.HubScope(hub))
	}
	collection := CollectionOf(envelope)
	if collection.IsZero() {
		// Every event about an entry carries its collection (api/events); one that does not is about
		// nothing in a hub.
		return path
	}
	path = append(path, identity.CollectionScope(collection))
	if item := itemOf(envelope); !item.IsZero() {
		// A share of the one entry reaches it as well.
		path = append(path, identity.ItemScope(item))
	}
	return path
}

// Placed reports whether the place names anything below the workspace.
func Placed(path []identity.Scope) bool { return len(path) > 1 }

// itemOf reads the entry an event is about: its `item_id`, or an `item/<id>` subject.
func itemOf(envelope event.Envelope) shared.ID {
	if id := idAt(envelope.Payload, "item_id"); !id.IsZero() {
		return id
	}
	if rest, found := strings.CutPrefix(envelope.Subject, "item/"); found {
		if id, err := shared.ParseID(rest); err == nil {
			return id
		}
	}
	return ""
}
