// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// LinkDirection says whether a link reads differently from its two ends.
type LinkDirection string

const (
	// LinkDirected reads one way from the source ("blocks") and another from the target
	// ("is blocked by").
	LinkDirected LinkDirection = "DIRECTED"
	// LinkSymmetric reads the same from both ends; its ends are stored in one order, so that A–B
	// and B–A are one link.
	LinkSymmetric LinkDirection = "SYMMETRIC"
)

// LinkCardinality bounds how many links of a type one entry starts.
type LinkCardinality string

const (
	// LinkMany lets an entry start any number of links of the type.
	LinkMany LinkCardinality = "MANY"
	// LinkOne lets an entry start at most one: an entry duplicates one other, not two.
	LinkOne LinkCardinality = "ONE"
)

// LinkCycles is the type's cycle rule.
type LinkCycles string

const (
	// LinkCyclesForbidden refuses a link that would close a loop: A blocks B blocks A is a plan
	// nobody can start.
	LinkCyclesForbidden LinkCycles = "FORBIDDEN"
	// LinkCyclesAllowed lets the graph loop: two incidents may affect each other.
	LinkCyclesAllowed LinkCycles = "ALLOWED"
)

// LinkWaits says which end of a link waits for the other (PLN-01). A waiting link is a dependency:
// it is drawn on the timeline, and the cycle rule of every waiting type is judged on one graph,
// because "A depends on B" and "A blocks B" together are a loop although they are two types.
type LinkWaits string

const (
	// LinkWaitsNone is a link nobody waits on: duplicates, affects, implements.
	LinkWaitsNone LinkWaits = "NONE"
	// LinkWaitsSource is "the source waits for the target": depends on.
	LinkWaitsSource LinkWaits = "SOURCE"
	// LinkWaitsTarget is "the target waits for the source": blocks.
	LinkWaitsTarget LinkWaits = "TARGET"
)

// The system link types (F-3). Declared here and never stored: a workspace has them without a row,
// and their words are the client's, by key (rule 8).
const (
	LinkTypeBlocks     = "blocks"
	LinkTypeDependsOn  = "depends_on"
	LinkTypeDuplicates = "duplicates"
	LinkTypeRunsOn     = "runs_on"
	LinkTypeAffects    = "affects"
	LinkTypeImplements = "implements"
)

var linkTypeKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,49}$`)

var linkNameCodes = nameCodes{
	empty:     "link_types.name_empty",
	tooLong:   "link_types.name_too_long",
	malformed: "link_types.name_malformed",
}

// LinkType is a kind of relation between two entries a workspace uses: the six Hubtask ships and
// the workspace's own (F-3, domain-model.md §3.5). What a link of a type means - its direction,
// cardinality, cycle rule and who waits - is fixed when the type is made, because changing it
// would make links already drawn wrong.
type LinkType struct {
	// ID is zero for a system type: it has no row.
	ID       shared.ID
	TenantID shared.ID
	Key      string
	System   bool
	// Name and InverseName are the workspace's words for its own type, read from the source and
	// from the target. Empty on a system type: a client shows its own words for the key.
	Name        string
	InverseName string
	Direction   LinkDirection
	Cardinality LinkCardinality
	Cycles      LinkCycles
	Waits       LinkWaits
	Position    int
	ArchivedAt  *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Version     int
}

// SystemLinkTypes are the six types every workspace has, in a picker's order.
func SystemLinkTypes() []LinkType {
	system := func(key string, cardinality LinkCardinality, cycles LinkCycles, waits LinkWaits, position int) LinkType {
		return LinkType{
			Key: key, System: true, Direction: LinkDirected, Cardinality: cardinality,
			Cycles: cycles, Waits: waits, Position: position,
		}
	}
	return []LinkType{
		system(LinkTypeBlocks, LinkMany, LinkCyclesForbidden, LinkWaitsTarget, 0),
		system(LinkTypeDependsOn, LinkMany, LinkCyclesForbidden, LinkWaitsSource, 1),
		system(LinkTypeDuplicates, LinkOne, LinkCyclesForbidden, LinkWaitsNone, 2),
		system(LinkTypeRunsOn, LinkMany, LinkCyclesForbidden, LinkWaitsNone, 3),
		system(LinkTypeAffects, LinkMany, LinkCyclesAllowed, LinkWaitsNone, 4),
		system(LinkTypeImplements, LinkMany, LinkCyclesForbidden, LinkWaitsNone, 5),
	}
}

// IsSystemLinkTypeKey reports whether the key is one of the six shipped types.
func IsSystemLinkTypeKey(key string) bool {
	for _, t := range SystemLinkTypes() {
		if t.Key == key {
			return true
		}
	}
	return false
}

// IsArchived reports whether the type is out of use: no new link of it, the old ones readable.
func (t LinkType) IsArchived() bool { return t.ArchivedAt != nil }

// Schedules reports whether a link of the type is a dependency one end waits on.
func (t LinkType) Schedules() bool { return t.Waits == LinkWaitsSource || t.Waits == LinkWaitsTarget }

// NewLinkTypeInput is what a workspace's own link type is made of.
type NewLinkTypeInput struct {
	ID          shared.ID
	TenantID    shared.ID
	Key         string
	Name        string
	InverseName string
	Direction   LinkDirection
	Cardinality LinkCardinality
	Cycles      LinkCycles
	Waits       LinkWaits
	Position    int
	Now         time.Time
	Text        text.Normalizer
}

// NewLinkType builds a workspace's own link type. A key already taken is the unique index's
// decision; a system key is refused here.
func NewLinkType(in NewLinkTypeInput) (LinkType, error) {
	if in.ID.IsZero() || in.TenantID.IsZero() {
		return LinkType{}, shared.ErrInternal.WithDetail("link_types.identity_incomplete")
	}
	key := strings.TrimSpace(in.Key)
	if !linkTypeKeyPattern.MatchString(key) {
		return LinkType{}, linkInvalid("/key", "link_types.key_malformed")
	}
	if IsSystemLinkTypeKey(key) {
		return LinkType{}, linkInvalid("/key", "link_types.key_reserved")
	}
	t := LinkType{
		ID: in.ID, TenantID: in.TenantID, Key: key,
		Direction: in.Direction, Cardinality: in.Cardinality, Cycles: in.Cycles, Waits: in.Waits,
		Position: in.Position, CreatedAt: in.Now, UpdatedAt: in.Now, Version: 1,
	}
	if t.Direction == "" {
		t.Direction = LinkDirected
	}
	if t.Cardinality == "" {
		t.Cardinality = LinkMany
	}
	if t.Cycles == "" {
		t.Cycles = LinkCyclesAllowed
	}
	if t.Waits == "" {
		t.Waits = LinkWaitsNone
	}
	switch {
	case !slices.Contains([]LinkDirection{LinkDirected, LinkSymmetric}, t.Direction):
		return LinkType{}, linkInvalid("/direction", "link_types.direction_unknown")
	case !slices.Contains([]LinkCardinality{LinkMany, LinkOne}, t.Cardinality):
		return LinkType{}, linkInvalid("/cardinality", "link_types.cardinality_unknown")
	case !slices.Contains([]LinkCycles{LinkCyclesForbidden, LinkCyclesAllowed}, t.Cycles):
		return LinkType{}, linkInvalid("/cycles", "link_types.cycles_unknown")
	case !slices.Contains([]LinkWaits{LinkWaitsNone, LinkWaitsSource, LinkWaitsTarget}, t.Waits):
		return LinkType{}, linkInvalid("/waits", "link_types.waits_unknown")
	case t.Direction == LinkSymmetric && t.Waits != LinkWaitsNone:
		// Who waits needs a direction: a symmetric link has no "first".
		return LinkType{}, linkInvalid("/waits", "link_types.symmetric_waits")
	case t.Direction == LinkSymmetric && t.Cardinality == LinkOne:
		return LinkType{}, linkInvalid("/cardinality", "link_types.symmetric_one")
	case t.Schedules() && t.Cycles != LinkCyclesForbidden:
		// A loop of dependencies is a plan nobody can start.
		return LinkType{}, linkInvalid("/cycles", "link_types.waiting_cycles")
	}
	return t.with(LinkTypeChange{Name: &in.Name, InverseName: &in.InverseName}, in.Text)
}

// LinkTypeChange is what a change of a workspace's own link type may set.
type LinkTypeChange struct {
	Name        *string
	InverseName *string
	Position    *int
	Archived    *bool
}

// IsEmpty reports whether the change asks for nothing.
func (c LinkTypeChange) IsEmpty() bool {
	return c.Name == nil && c.InverseName == nil && c.Position == nil && c.Archived == nil
}

// Changed applies a change at now and reports which fields moved. A system type does not change.
func (t LinkType) Changed(change LinkTypeChange, now time.Time, form text.Normalizer) (LinkType, []FieldChange, error) {
	if t.System {
		return LinkType{}, nil, shared.ErrConflict.WithDetail("link_types.system_fixed")
	}
	if change.IsEmpty() {
		return LinkType{}, nil, shared.ErrValidation.WithDetail("link_types.update_empty")
	}
	next, err := t.with(change, form)
	if err != nil {
		return LinkType{}, nil, err
	}
	if change.Position != nil {
		next.Position = *change.Position
	}
	if change.Archived != nil {
		switch {
		case *change.Archived && next.ArchivedAt == nil:
			at := now
			next.ArchivedAt = &at
		case !*change.Archived:
			next.ArchivedAt = nil
		}
	}
	var changes []FieldChange
	add := func(field, from, to string) {
		if from != to {
			changes = append(changes, FieldChange{Field: field, From: from, To: to})
		}
	}
	add(FieldName, t.Name, next.Name)
	add("inverse_name", t.InverseName, next.InverseName)
	add("position", strconv.Itoa(t.Position), strconv.Itoa(next.Position))
	add("archived", boolCode(t.IsArchived()), boolCode(next.IsArchived()))
	if len(changes) > 0 {
		next.UpdatedAt = now
	}
	return next, changes, nil
}

func (t LinkType) with(change LinkTypeChange, form text.Normalizer) (LinkType, error) {
	next := t
	if change.Name != nil {
		name, err := structureName(*change.Name, linkNameCodes, form)
		if err != nil {
			return LinkType{}, err
		}
		next.Name = name
	}
	if change.InverseName != nil {
		raw := strings.TrimSpace(*change.InverseName)
		switch {
		case raw == "" && next.Direction == LinkSymmetric:
			// Read the same from both ends.
			next.InverseName = ""
		case raw == "":
			return LinkType{}, linkInvalid("/inverse_name", "link_types.inverse_name_empty")
		default:
			name, err := structureName(raw, linkNameCodes, form)
			if err != nil {
				return LinkType{}, err
			}
			next.InverseName = name
		}
	}
	return next, nil
}

// LinkTypes is a workspace's registry: the system types and its own.
type LinkTypes []LinkType

// NewLinkTypes merges the workspace's rows with the system types, the system types first.
func NewLinkTypes(own []LinkType) LinkTypes {
	registry := LinkTypes(SystemLinkTypes())
	for _, t := range own {
		if !IsSystemLinkTypeKey(t.Key) {
			registry = append(registry, t)
		}
	}
	return registry
}

// Find returns the type of a key.
func (r LinkTypes) Find(key string) (LinkType, bool) {
	for _, t := range r {
		if t.Key == key {
			return t, true
		}
	}
	return LinkType{}, false
}

// Waiting are the keys of the dependency types, split by which end waits: the graph the cycle
// rule of every waiting type is judged on.
func (r LinkTypes) Waiting() (sourceWaits, targetWaits []string) {
	sourceWaits, targetWaits = []string{}, []string{}
	for _, t := range r {
		switch t.Waits {
		case LinkWaitsSource:
			sourceWaits = append(sourceWaits, t.Key)
		case LinkWaitsTarget:
			targetWaits = append(targetWaits, t.Key)
		}
	}
	return sourceWaits, targetWaits
}

// EntryLink is one relation of a type from one entry to another (F-3, PLN-01). It is drawn and
// removed, never changed: a different relation is a different link.
type EntryLink struct {
	ID        shared.ID
	TenantID  shared.ID
	Type      string
	SourceID  shared.ID
	TargetID  shared.ID
	CreatedBy shared.ID
	CreatedAt time.Time
	// Timing is how the waiting end hangs on the other (M2): finish-to-start without a lag unless
	// a planner says otherwise. Only a dependency carries another.
	Timing planning.Timing
}

// NewEntryLinkInput is what a link is made of.
type NewEntryLinkInput struct {
	ID        shared.ID
	TenantID  shared.ID
	Type      LinkType
	SourceID  shared.ID
	TargetID  shared.ID
	CreatedBy shared.ID
	Now       time.Time
	Timing    planning.Timing
}

// NewEntryLink checks a link's own invariants. The ones that need the graph - the cardinality and
// the cycle rule - are the use case's, under the type's lock.
func NewEntryLink(in NewEntryLinkInput) (EntryLink, error) {
	if in.ID.IsZero() || in.TenantID.IsZero() {
		return EntryLink{}, shared.ErrInternal.WithDetail("links.identity_incomplete")
	}
	if in.SourceID.IsZero() {
		return EntryLink{}, linkInvalid("/source_id", "links.source_required")
	}
	if in.TargetID.IsZero() {
		return EntryLink{}, linkInvalid("/target_id", "links.target_required")
	}
	if in.SourceID == in.TargetID {
		return EntryLink{}, linkInvalid("/target_id", "links.self")
	}
	if in.Type.IsArchived() {
		return EntryLink{}, linkInvalid("/type", "links.type_archived")
	}
	timing, err := in.Type.TimingOf(in.Timing)
	if err != nil {
		return EntryLink{}, err
	}
	source, target := in.SourceID, in.TargetID
	if in.Type.Direction == LinkSymmetric && target.String() < source.String() {
		source, target = target, source
	}
	return EntryLink{
		ID: in.ID, TenantID: in.TenantID, Type: in.Type.Key, SourceID: source, TargetID: target,
		CreatedBy: in.CreatedBy, CreatedAt: in.Now, Timing: timing,
	}, nil
}

// TimingOf checks a timing for a link of the type: a dependency takes any the planning module
// accepts, a link nobody waits on only the default - a lag on "duplicates" would mean nothing.
func (t LinkType) TimingOf(timing planning.Timing) (planning.Timing, error) {
	checked, err := planning.NewTiming(timing.Kind, timing.LagDays)
	if err != nil {
		return planning.Timing{}, err
	}
	if !t.Schedules() && checked != (planning.Timing{Kind: planning.FinishToStart}) {
		return planning.Timing{}, linkInvalid("/dependency_kind", "links.timing_without_dependency")
	}
	return checked, nil
}

// Other is the end of the link that is not the entry, and whether the entry is its source.
func (l EntryLink) Other(entry shared.ID) (shared.ID, bool) {
	if l.SourceID == entry {
		return l.TargetID, true
	}
	return l.SourceID, false
}

// Dependency is the link as "waiter waits for waitedFor", for a type that waits.
func (l EntryLink) Dependency(t LinkType) (waiter, waitedFor shared.ID, ok bool) {
	switch t.Waits {
	case LinkWaitsSource:
		return l.SourceID, l.TargetID, true
	case LinkWaitsTarget:
		return l.TargetID, l.SourceID, true
	}
	return "", "", false
}

// ClosesLoopFrom names the walk that would prove a loop: from which entry, to which, over the
// edges of which types read forward (source to target) and which backward. A new link closes a
// loop when the walk arrives. Over the waiting graph for a dependency, over its own type else.
func (l EntryLink) ClosesLoopFrom(t LinkType, registry LinkTypes) (from, to shared.ID, forward, backward []string) {
	if t.Schedules() {
		// Edges read as "waiter -> waited for": depends_on forward, blocks backward.
		forward, backward = registry.Waiting()
		waiter, waitedFor, _ := l.Dependency(t)
		return waitedFor, waiter, forward, backward
	}
	if t.Direction == LinkSymmetric {
		// Undirected: the two ends already connected is the loop.
		return l.TargetID, l.SourceID, []string{t.Key}, []string{t.Key}
	}
	return l.TargetID, l.SourceID, []string{t.Key}, []string{}
}

func linkInvalid(path, code string) error {
	return shared.ErrValidation.WithDetail(code).WithFields(shared.FieldError{Path: path, Code: code})
}
