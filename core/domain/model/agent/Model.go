// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package agent

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// Source is where a registered model is reached: the workspace's own provider, or a model the
// installation offers (ai-first.md §2, ADR-0072).
type Source string

const (
	SourceOwn     Source = "OWN"
	SourceOffered Source = "OFFERED"
)

// Region is where a model is hosted, on the scale ai.min_jurisdiction uses.
type Region string

const (
	RegionAny        Region = "ANY"
	RegionAdequacy   Region = "ADEQUACY"
	RegionEEA        Region = "EEA"
	RegionSelfHosted Region = "SELF_HOSTED"
)

func (r Region) rank() int {
	switch r {
	case RegionAdequacy:
		return 1
	case RegionEEA:
		return 2
	case RegionSelfHosted:
		return 3
	}
	return 0
}

// AtLeast reports a region no weaker than the minimum.
func (r Region) AtLeast(min Region) bool { return r.rank() >= min.rank() }

// Model is a model the workspace registered for its agents (G9): the source it is reached
// through, its name there, where it is hosted, how long the provider keeps what it is sent, and
// who consented to sending it - the consent the registration is. A change of any of them is a new
// revision, which every agent using it must be approved for again.
type Model struct {
	ID       shared.ID
	TenantID shared.ID
	Key      string
	Name     string
	Source   Source
	// OfferedID is the installation's model, for SourceOffered.
	OfferedID shared.ID
	// Model is the model's name at its provider.
	Model  string
	Region Region
	// RetentionDays is how long the provider keeps what it is sent; nil is not known, zero is not
	// at all.
	RetentionDays *int
	ConsentBy     shared.ID
	ConsentAt     time.Time
	Revision      int
	Retired       bool
	UpdatedAt     time.Time
	Version       int
}

// CheckModel checks a model's fields as it is registered or changed.
func CheckModel(m Model, normalizer text.Normalizer) (Model, error) {
	m.Key = strings.TrimSpace(m.Key)
	if !keyPattern(m.Key) {
		return Model{}, invalid("/key", "agent_models.key_invalid")
	}
	name := strings.TrimSpace(m.Name)
	if normalizer != nil {
		name = normalizer.NFC(name)
	}
	if name == "" || utf8.RuneCountInString(name) > MaxNameLength {
		return Model{}, invalid("/name", "agent_models.name_invalid")
	}
	m.Name = name
	switch m.Source {
	case SourceOwn:
		m.OfferedID = ""
	case SourceOffered:
		if m.OfferedID.IsZero() {
			return Model{}, invalid("/offered_id", "agent_models.offered_required")
		}
	default:
		return Model{}, invalid("/source", "agent_models.source_invalid")
	}
	m.Model = strings.TrimSpace(m.Model)
	if m.Model == "" || len(m.Model) > 200 {
		return Model{}, invalid("/model", "agent_models.model_invalid")
	}
	switch m.Region {
	case RegionAny, RegionAdequacy, RegionEEA, RegionSelfHosted:
	default:
		return Model{}, invalid("/region", "agent_models.region_invalid")
	}
	if m.RetentionDays != nil && (*m.RetentionDays < 0 || *m.RetentionDays > 3650) {
		return Model{}, invalid("/retention_days", "agent_models.retention_invalid")
	}
	return m, nil
}

// Revised answers the model after a change: a new revision where anything an approval relied on
// changed, the same one where only its name did.
func (m Model) Revised(next Model, by shared.ID, at time.Time) Model {
	next.ID, next.TenantID, next.Key, next.Version = m.ID, m.TenantID, m.Key, m.Version
	next.Revision, next.ConsentBy, next.ConsentAt = m.Revision, m.ConsentBy, m.ConsentAt
	same := m.Source == next.Source && m.OfferedID == next.OfferedID && m.Model == next.Model &&
		m.Region == next.Region && sameDays(m.RetentionDays, next.RetentionDays)
	if !same {
		next.Revision = m.Revision + 1
		next.ConsentBy, next.ConsentAt = by, at
	}
	return next
}

func sameDays(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// keyPattern is a block's stable key: lower case letters, digits, dashes and underscores.
func keyPattern(key string) bool {
	if key == "" || len(key) > 64 {
		return false
	}
	for i, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9' && i > 0, (r == '-' || r == '_') && i > 0:
		default:
			return false
		}
	}
	return true
}
