// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package shared

import "slices"

// ActorKind is who is acting: the vocabulary of audit.md §2, and the one an event, an audit entry
// and a request context all speak.
//
// It lives here rather than in the layer that first needed it, because three copies of the same
// five words is how an audit trail comes to record `AI_AGENT` while an event says `agent`. A port
// may import this package and nothing else of the domain (project-structure.md §2), which is
// exactly what a shared vocabulary has to satisfy.
type ActorKind string

const (
	// ActorAnonymous is a request that carried no credential. It never reaches an audit entry as
	// an actor - an unauthenticated request performs no auditable action - but a refused one is
	// recorded with outcome DENIED, and it needs a name for that.
	ActorAnonymous      ActorKind = "ANONYMOUS"
	ActorUser           ActorKind = "USER"
	ActorServiceAccount ActorKind = "SERVICE_ACCOUNT"
	ActorAutomation     ActorKind = "AUTOMATION"
	ActorAIAgent        ActorKind = "AI_AGENT"
	// ActorSystem is the installation acting for itself: a scheduled job, a retention run, a
	// migration.
	ActorSystem ActorKind = "SYSTEM"
)

// actorKinds is the closed set, in the order of the constants above.
var actorKinds = [...]ActorKind{
	ActorAnonymous, ActorUser, ActorServiceAccount, ActorAutomation, ActorAIAgent, ActorSystem,
}

// ActorKinds returns every defined kind.
func ActorKinds() []ActorKind { return actorKinds[:] }

// Valid reports whether the kind is one of the defined ones.
func (k ActorKind) Valid() bool { return slices.Contains(actorKinds[:], k) }

// Auditable reports whether the kind may appear as the actor of an audit entry. The database
// constrains the same five values (db/schema.sql, audit_log.actor_type), so an entry written with
// ANONYMOUS would be refused at the boundary - this is what refuses it before the transaction.
func (k ActorKind) Auditable() bool { return k.Valid() && k != ActorAnonymous }
