// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"strconv"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// TemplateOrigin is who keeps a template (configuration.md §7).
type TemplateOrigin string

const (
	OriginHubtask      TemplateOrigin = "HUBTASK"
	OriginInstallation TemplateOrigin = "INSTALLATION"
	OriginWorkspace    TemplateOrigin = "WORKSPACE"
	OriginOwn          TemplateOrigin = "OWN"
)

// Template is one version of a bundle of building blocks (D-17): a document read in MERGE mode,
// so taking it adds and changes what it names and leaves the rest of the place alone. A version
// never changes once published; a change is the next version.
type Template struct {
	ID       shared.ID
	TenantID shared.ID
	Key      string
	Version  int
	Name     string
	Origin   TemplateOrigin
	Document Document
	// Hub is the part a hub takes (configuration.md §6, a hub template): a hub's own document - its
	// switches, structure, scheme and calendars - beside the workspace's part; nil for a template
	// of the workspace only.
	Hub Document
	// Questions are what applying the template asks (Answer); only Hubtask's own ask any yet.
	Questions []Question
	CreatedBy shared.ID
	CreatedAt time.Time
}

// ForHubs reports whether a hub can be set up from the template.
func (t Template) ForHubs() bool { return t.Hub != nil }

// Provenance is how a place records what it was set up from: `key@version`.
func (t Template) Provenance() string { return Provenance(t.Key, t.Version) }

// Provenance spells a template's key and version.
func Provenance(key string, version int) string { return key + "@" + strconv.Itoa(version) }

// TemplateUse is a template a workspace or a hub took: the version, and the template's document as
// it was then - the hub's part for a hub - the base of a three-way upgrade.
type TemplateUse struct {
	// HubID is the hub that took the template's hub part, empty for the workspace.
	HubID       shared.ID
	TemplateKey string
	Version     int
	Document    Document
	At          time.Time
}

// UpgradeState is one attribute's place in a three-way comparison.
type UpgradeState string

const (
	// UpgradeTake: the template changed it and the place did not - taken by default.
	UpgradeTake UpgradeState = "TAKE"
	// UpgradeConflict: the template changed it and so did the place, differently - shown, left
	// unless the person takes it.
	UpgradeConflict UpgradeState = "CONFLICT"
)

// UpgradeChange is one attribute (or, for a block the new version drops, one block) a newer
// template version changes.
type UpgradeChange struct {
	Section   string
	Key       string
	Attribute string
	Base      any
	Ours      any
	Theirs    any
	State     UpgradeState
	// Removed is true when the newer version no longer has the block.
	Removed bool
}

// Path is the change's key path: `types.incident.name`, or `types.incident` for a block.
func (u UpgradeChange) Path() string {
	path := u.Section + "." + u.Key
	if u.Attribute != "" {
		path += "." + u.Attribute
	}
	return path
}

// ThreeWay compares what a template changed between the version a place took (base) and a newer
// one (theirs) with what the place has now (ours). A change the place made itself is never
// overwritten without being shown: it is a conflict, not a take.
func ThreeWay(base, ours, theirs Document) []UpgradeChange {
	var out []UpgradeChange
	for _, name := range sortedKeys(base, theirs) {
		for _, key := range sortedKeys(base[name], theirs[name]) {
			before, inBase := base[name][key]
			after, inTheirs := theirs[name][key]
			current, inOurs := ours[name][key]
			if inBase && !inTheirs {
				if !inOurs {
					continue
				}
				state := UpgradeTake
				if len(differing(projection(current, before), before)) > 0 {
					state = UpgradeConflict
				}
				out = append(out, UpgradeChange{Section: name, Key: key, State: state, Removed: true})
				continue
			}
			for _, attribute := range sortedKeys(before, after) {
				b, t := before[attribute], after[attribute]
				if Canonical(b) == Canonical(t) {
					continue
				}
				var o any
				if inOurs {
					o = current[attribute]
				}
				switch {
				case Canonical(o) == Canonical(t):
					continue
				case Canonical(o) == Canonical(b):
					out = append(out, UpgradeChange{Section: name, Key: key, Attribute: attribute, Base: b, Ours: o, Theirs: t, State: UpgradeTake})
				default:
					out = append(out, UpgradeChange{Section: name, Key: key, Attribute: attribute, Base: b, Ours: o, Theirs: t, State: UpgradeConflict})
				}
			}
		}
	}
	return out
}

// projection is a block narrowed to the attributes another names: what a template set of it.
func projection(block, names Block) Block {
	out := Block{}
	for name := range names {
		if value, held := block[name]; held {
			out[name] = value
		}
	}
	return out
}

// UpgradeDocument is the MERGE document that takes the chosen changes: those named by path, or
// every TAKE when none is named.
func UpgradeDocument(changes []UpgradeChange, chosen []string) Document {
	pick := map[string]bool{}
	for _, path := range chosen {
		pick[path] = true
	}
	out := Document{}
	for _, change := range changes {
		if (len(chosen) == 0 && change.State != UpgradeTake) || (len(chosen) > 0 && !pick[change.Path()]) {
			continue
		}
		section := out[change.Section]
		if section == nil {
			section = Section{}
			out[change.Section] = section
		}
		if change.Removed {
			section[change.Key] = nil
			continue
		}
		block := section[change.Key]
		if block == nil {
			block = Block{}
			section[change.Key] = block
		}
		block[change.Attribute] = change.Theirs
	}
	return out
}

// ErrTemplateUnknown refuses a template or version the workspace does not have.
func ErrTemplateUnknown(key string) error {
	return shared.ErrNotFound.WithDetail("config.template_unknown").WithParams(map[string]string{"template": key})
}

// ErrTemplateNotTaken refuses an upgrade of a template the workspace never took.
func ErrTemplateNotTaken(key string) error {
	return shared.ErrValidation.WithDetail("config.template_not_taken").WithParams(map[string]string{"template": key}).
		WithFields(shared.FieldError{Path: "/template", Code: "config.template_not_taken"})
}
