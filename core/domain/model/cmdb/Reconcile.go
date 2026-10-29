// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The bounds of one reconciliation.
const (
	MaxPayloadItems     = 2000
	MaxPayloadRelations = 5000
)

// SourceKind says how a source reaches the reconciliation.
type SourceKind string

const (
	// SourceManual is a person in any door.
	SourceManual SourceKind = "MANUAL"
	// SourceFile is a file import.
	SourceFile SourceKind = "FILE"
	// SourceAPI is a payload sent by a program.
	SourceAPI SourceKind = "API"
	// SourceConnector is a connector's pull or push (S-17).
	SourceConnector SourceKind = "CONNECTOR"
)

// ManualSource is the key of the shipped source every person writes as.
const ManualSource = "manual"

// Source is one place CIs come from, with how far it is trusted (research itsm-cmdb-ai R2).
type Source struct {
	ID       shared.ID
	TenantID shared.ID
	Key      string
	Kind     SourceKind
	// Rank is the source's precedence: a lower number wins an attribute a higher one last wrote.
	Rank int
	// Classes are the classes it may write, with their descendants; empty is every class.
	Classes []string
	// MayCreate lets the source make new CIs; without it, it only updates what it identifies.
	MayCreate bool
	// Mapping is how a file of this source is read (FILE sources).
	Mapping *FileMapping
	System  bool
	// LastRunAt is when it last ran.
	LastRunAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
	Version   int
}

// SystemSources is the source Hubtask ships: people, ranked first.
func SystemSources() []Source {
	return []Source{{Key: ManualSource, Kind: SourceManual, Rank: 10, MayCreate: true, System: true}}
}

// SourceInput is what a workspace writes for a source.
type SourceInput struct {
	ID        shared.ID
	TenantID  shared.ID
	Key       string
	Kind      SourceKind
	Rank      int
	Classes   []string
	MayCreate bool
	Mapping   *FileMapping
	Now       time.Time
}

// NewSource checks a source.
func NewSource(in SourceInput, existing *Source, classes Classes) (Source, error) {
	if !keyPattern.MatchString(in.Key) {
		return Source{}, invalid("/key", "cmdb.source_key_malformed")
	}
	if in.Key == ManualSource {
		return Source{}, shared.ErrConflict.WithDetail("cmdb.source_key_taken").WithParams(map[string]string{"key": in.Key})
	}
	if !slices.Contains([]SourceKind{SourceFile, SourceAPI, SourceConnector}, in.Kind) {
		return Source{}, invalid("/kind", "cmdb.source_kind_unknown")
	}
	if in.Rank < 1 || in.Rank > 1000 {
		return Source{}, invalid("/rank", "cmdb.source_rank_invalid")
	}
	for i, key := range in.Classes {
		if _, ok := classes.Find(key); !ok {
			return Source{}, invalid("/classes/"+strconv.Itoa(i), "cmdb.class_unknown", "key", key)
		}
	}
	if in.Mapping != nil {
		if err := in.Mapping.Check(classes); err != nil {
			return Source{}, err
		}
	}
	s := Source{ID: in.ID, TenantID: in.TenantID, CreatedAt: in.Now, Version: 1}
	if existing != nil {
		s = *existing
		s.Version++
	}
	s.Key, s.Kind, s.Rank, s.Classes, s.MayCreate, s.Mapping, s.UpdatedAt = in.Key, in.Kind, in.Rank, in.Classes, in.MayCreate, in.Mapping, in.Now
	return s, nil
}

// Writes reports whether the source may write a class.
func (s Source) Writes(class string, classes Classes) bool {
	if len(s.Classes) == 0 {
		return true
	}
	for _, c := range s.Classes {
		if classes.IsA(class, c) {
			return true
		}
	}
	return false
}

// Item is one CI as a source reports it: the single payload shape of every source (R1).
type Item struct {
	// Ref names the item within its payload, for its relations.
	Ref   string
	Class string
	Name  string
	// NativeKey is the source's own identifier of the CI; it is matched first (R4).
	NativeKey  string
	Attributes map[string]any
}

// RelationItem is a relation as a source reports it, between two of its items (by Ref) or
// existing CIs (by identifier).
type RelationItem struct {
	Type   string
	From   string
	To     string
	FromID shared.ID
	ToID   shared.ID
}

// Payload is one batch a source sends.
type Payload struct {
	Items     []Item
	Relations []RelationItem
}

// Outcome is what the reconciliation did with an item.
type Outcome string

const (
	OutcomeCreate    Outcome = "CREATE"
	OutcomeUpdate    Outcome = "UPDATE"
	OutcomeUnchanged Outcome = "UNCHANGED"
	// OutcomeHold keeps an item that cannot be identified, or that the source may not create, in
	// review rather than inserting a CI nobody can find again (R6).
	OutcomeHold Outcome = "HOLD"
	// OutcomeDuplicate is an item that matched more than one CI: nothing is written and a
	// duplicate to merge is opened (R7).
	OutcomeDuplicate Outcome = "DUPLICATE"
	// OutcomeRefused is an item that breaks its class (an unknown class or attribute, a value of
	// the wrong kind).
	OutcomeRefused Outcome = "REFUSED"
)

// Kept is an attribute the source sent and the reconciliation did not take, and who holds it.
type Kept struct {
	Attribute string
	By        string
}

// Decision explains what happened to one item (R8): the rule that matched, what was taken and
// what was kept from whom. It names attributes, never their values.
type Decision struct {
	Ref        string
	Outcome    Outcome
	CI         shared.ID
	Class      string
	Rule       string
	Taken      []string
	Kept       []Kept
	Candidates []shared.ID
	Reason     string
}

// IdentifierRow is one entry of the identifier index the application looked up.
type IdentifierRow struct {
	CI    shared.ID
	Class string
	Name  string
	Value string
}

// Identification is which CIs an item matched and by what.
type Identification struct {
	// Rule is `native_key`, or the matching rule's attributes joined by `+`; empty when no rule
	// could be tried.
	Rule    string
	Matches []shared.ID
	// Usable is false when the item carries neither a native key nor every attribute of any rule.
	Usable bool
}

// ItemIdentifiers answers an item's normalised identifiers keyed as the index keeps them.
func ItemIdentifiers(item Item, class Resolved, source string) map[string]string {
	ci := CI{Name: item.Name, Attributes: item.Attributes}
	out := ci.Identifiers(class)
	if key := strings.TrimSpace(item.NativeKey); key != "" {
		out[NativeKey(source)] = strings.ToLower(key)
	}
	return out
}

// Identify matches an item against the index rows found for its identifiers: its native key
// first, then the class's rules in order, each among the CIs of the class that declared it and
// that class's descendants (R3, R4).
func Identify(class Resolved, ids map[string]string, source string, rows []IdentifierRow, classes Classes) Identification {
	hits := func(name, value string, within string) map[shared.ID]bool {
		out := map[shared.ID]bool{}
		for _, r := range rows {
			if r.Name == name && r.Value == value && (within == "" || classes.IsA(r.Class, within)) {
				out[r.CI] = true
			}
		}
		return out
	}
	if native, ok := ids[NativeKey(source)]; ok {
		if found := hits(NativeKey(source), native, ""); len(found) > 0 {
			return Identification{Rule: "native_key", Matches: sorted(found), Usable: true}
		}
	}
	result := Identification{Usable: ids[NativeKey(source)] != ""}
	for _, rule := range class.Rules {
		var found map[shared.ID]bool
		complete := true
		for _, key := range rule.Attributes {
			value, ok := ids[key]
			if !ok {
				complete = false
				break
			}
			these := hits(key, value, rule.Class)
			if found == nil {
				found = these
				continue
			}
			for id := range found {
				if !these[id] {
					delete(found, id)
				}
			}
		}
		if !complete {
			continue
		}
		result.Usable = true
		if len(found) > 0 {
			return Identification{Rule: strings.Join(rule.Attributes, "+"), Matches: sorted(found), Usable: true}
		}
	}
	return result
}

func sorted(set map[shared.ID]bool) []shared.ID {
	out := slices.Collect(maps.Keys(set))
	slices.Sort(out)
	return out
}

// Precedence answers the rank of every source by key.
type Precedence map[string]int

// mayWrite reports whether a source may write an attribute another wrote last: the same source,
// one of higher or equal precedence, or any source once the last writer has gone silent for the
// class's staleness period (R5, staleness takeover).
func (p Precedence) mayWrite(source string, last Provenance, ok bool, stale time.Duration, now time.Time) bool {
	if !ok || last.Source == source {
		return true
	}
	rank, known := p[last.Source]
	if !known || p[source] <= rank {
		return true
	}
	return now.Sub(last.At) > stale
}

// Apply writes an item into a CI under the source's precedence and answers what it took and
// what it kept. A new CI is the zero CI; its name is the item's.
func Apply(ci CI, item Item, class Resolved, source string, run shared.ID, precedence Precedence, now time.Time) (CI, []string, []Kept, error) {
	next := ci
	next.Attributes = maps.Clone(ci.Attributes)
	if next.Attributes == nil {
		next.Attributes = map[string]any{}
	}
	next.Provenance = maps.Clone(ci.Provenance)
	if next.Provenance == nil {
		next.Provenance = map[string]Provenance{}
	}
	stale := time.Duration(class.StalenessDays) * 24 * time.Hour
	var taken []string
	var kept []Kept
	write := func(key string, value any, set func()) {
		last, ok := next.Provenance[key]
		if !precedence.mayWrite(source, last, ok, stale, now) {
			kept = append(kept, Kept{Attribute: key, By: last.Source})
			return
		}
		next.Provenance[key] = Provenance{Source: source, Run: run, At: now}
		if !reflect.DeepEqual(current(next, key), value) {
			set()
			taken = append(taken, key)
		}
	}
	if name := strings.TrimSpace(item.Name); name != "" {
		if err := CheckName(name); err != nil {
			return CI{}, nil, nil, err
		}
		write(FieldName, name, func() { next.Name = name })
	}
	for _, key := range slices.Sorted(maps.Keys(item.Attributes)) {
		a, ok := class.Attribute(key)
		if !ok {
			return CI{}, nil, nil, invalid("/attributes/"+key, "cmdb.attribute_unknown", "key", key)
		}
		value, err := NormaliseValue(a, item.Attributes[key])
		if err != nil {
			return CI{}, nil, nil, err
		}
		write(key, value, func() {
			if value == nil {
				delete(next.Attributes, key)
			} else {
				next.Attributes[key] = value
			}
		})
	}
	if next.Name == "" {
		return CI{}, nil, nil, invalid("/name", "cmdb.ci_name_invalid")
	}
	next.LastSeen = &now
	return next, taken, kept, nil
}

func current(ci CI, key string) any {
	if key == FieldName {
		return ci.Name
	}
	return ci.Attributes[key]
}

// Merge folds a duplicate into its survivor: the survivor keeps its identity and its own values,
// and takes the attributes it lacks; the duplicate is retired as merged into it, never deleted
// (R7 - deleting a CI is the owner's decision).
func Merge(survivor, duplicate CI, now time.Time, by shared.ID) (CI, CI) {
	s := survivor
	s.Attributes = maps.Clone(survivor.Attributes)
	s.Provenance = maps.Clone(survivor.Provenance)
	if s.Attributes == nil {
		s.Attributes = map[string]any{}
	}
	if s.Provenance == nil {
		s.Provenance = map[string]Provenance{}
	}
	for key, value := range duplicate.Attributes {
		if _, has := s.Attributes[key]; !has {
			s.Attributes[key] = value
			if p, ok := duplicate.Provenance[key]; ok {
				s.Provenance[key] = p
			}
		}
	}
	if duplicate.LastSeen != nil && (s.LastSeen == nil || duplicate.LastSeen.After(*s.LastSeen)) {
		s.LastSeen = duplicate.LastSeen
	}
	s.UpdatedAt, s.UpdatedBy = now, by
	d := duplicate
	d.Stage, d.Status, d.MergedInto, d.UpdatedAt, d.UpdatedBy = StageRetired, StatusMerged, survivor.ID, now, by
	return s, d
}
