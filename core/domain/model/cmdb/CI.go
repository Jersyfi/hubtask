// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"math"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// MaxCIsPerHub bounds a hub's inventory; health and the file import read it whole.
const MaxCIsPerHub = 20000

// Stage is where a CI is in its life, the same for every class (research itsm-cmdb-ai D4).
type Stage string

const (
	StagePlanned     Stage = "PLANNED"
	StageOrdered     Stage = "ORDERED"
	StageInStock     Stage = "IN_STOCK"
	StageInUse       Stage = "IN_USE"
	StageMaintenance Stage = "MAINTENANCE"
	StageRetired     Stage = "RETIRED"
	StageDisposed    Stage = "DISPOSED"
)

// Stages is every stage in the order of a life.
var Stages = []Stage{StagePlanned, StageOrdered, StageInStock, StageInUse, StageMaintenance, StageRetired, StageDisposed}

// Status qualifies the stage.
type Status string

const (
	StatusOK      Status = "OK"
	StatusPending Status = "PENDING"
	StatusBlocked Status = "BLOCKED"
	// StatusMerged marks a duplicate merged into its survivor: kept retired, never deleted.
	StatusMerged Status = "MERGED"
)

// Statuses is every status.
var Statuses = []Status{StatusOK, StatusPending, StatusBlocked, StatusMerged}

// Active reports whether the stage counts in health: a retired or disposed CI is history.
func (s Stage) Active() bool { return s != StageRetired && s != StageDisposed }

// Level is a protection need (BSI 200-2): normal, high, very high.
type Level string

const (
	LevelNone     Level = ""
	LevelNormal   Level = "NORMAL"
	LevelHigh     Level = "HIGH"
	LevelVeryHigh Level = "VERY_HIGH"
)

func (l Level) rank() int {
	return map[Level]int{LevelNormal: 1, LevelHigh: 2, LevelVeryHigh: 3}[l]
}

// Max answers the higher need.
func Max(a, b Level) Level {
	if b.rank() > a.rank() {
		return b
	}
	return a
}

// Need is a protection need for confidentiality, integrity and availability.
type Protection struct {
	C Level
	I Level
	A Level
}

// Max answers the higher need per dimension.
func (p Protection) Max(o Protection) Protection {
	return Protection{C: Max(p.C, o.C), I: Max(p.I, o.I), A: Max(p.A, o.A)}
}

// OwnProtection is what a person set on a CI. A dimension marked fixed is not raised by what
// depends on the CI - BSI's distribution effect, which needs a reason.
type OwnProtection struct {
	Protection
	Fixed  []string
	Reason string
}

// Compliance is the CI's fields that answer ISO/IEC 27001 A.5.9, NIS2 CIR §12.4 and BSI 200-2 at
// once: an owner, a location, a protection need, a review date and a quantity for a group of
// alike objects.
type Compliance struct {
	OwnerAccount shared.ID
	OwnerGroup   shared.ID
	// Location is a CI of a location class.
	Location   shared.ID
	Protection OwnProtection
	// ReviewDue is the day by which the CI is reviewed again (NIS2 CIR §12.5: at least yearly).
	ReviewDue *time.Time
	// Quantity counts alike objects recorded as one (BSI 200-2 §8.1 grouping); 1 is one object.
	Quantity int
}

// Provenance is who last wrote an attribute: the source, its run and when.
type Provenance struct {
	Source string
	Run    shared.ID
	At     time.Time
}

// CI is a configuration item.
type CI struct {
	ID         shared.ID
	TenantID   shared.ID
	HubID      shared.ID
	Class      string
	Name       string
	Stage      Stage
	Status     Status
	Attributes map[string]any
	// Provenance is per attribute (and `name`); the reconciliation's precedence reads it.
	Provenance map[string]Provenance
	Compliance Compliance
	// LastSeen is when a source last confirmed the CI; staleness reads it.
	LastSeen *time.Time
	// MergedInto is the survivor a merged duplicate went into.
	MergedInto shared.ID
	CreatedBy  shared.ID
	CreatedAt  time.Time
	UpdatedBy  shared.ID
	UpdatedAt  time.Time
	Version    int
}

// The fields a change of a CI names (audit, history).
const (
	FieldName       = "name"
	FieldClass      = "class"
	FieldLifecycle  = "lifecycle"
	FieldCompliance = "compliance"
)

// CheckName refuses an empty, multi-line or too long name.
func CheckName(name string) error {
	if name == "" || len([]rune(name)) > MaxNameLength || strings.ContainsAny(name, "\n\r") {
		return invalid("/name", "cmdb.ci_name_invalid")
	}
	return nil
}

// CheckLifecycle refuses an unknown stage or status.
func CheckLifecycle(stage Stage, status Status) error {
	if !slices.Contains(Stages, stage) {
		return invalid("/stage", "cmdb.stage_unknown")
	}
	if !slices.Contains(Statuses, status) {
		return invalid("/status", "cmdb.status_unknown")
	}
	return nil
}

// CheckCompliance refuses an unknown level, a fixed dimension without a reason and a quantity
// below one.
func CheckCompliance(c Compliance) error {
	for path, l := range map[string]Level{"/protection/c": c.Protection.C, "/protection/i": c.Protection.I, "/protection/a": c.Protection.A} {
		if l != LevelNone && l.rank() == 0 {
			return invalid(path, "cmdb.protection_level_unknown")
		}
	}
	for _, d := range c.Protection.Fixed {
		if d != "c" && d != "i" && d != "a" {
			return invalid("/protection/fixed", "cmdb.protection_dimension_unknown")
		}
	}
	if len(c.Protection.Fixed) > 0 && strings.TrimSpace(c.Protection.Reason) == "" {
		return invalid("/protection/reason", "cmdb.protection_reason_required")
	}
	if len([]rune(c.Protection.Reason)) > 2000 {
		return invalid("/protection/reason", "cmdb.protection_reason_too_long")
	}
	if c.Quantity < 1 || c.Quantity > 1000000 {
		return invalid("/quantity", "cmdb.quantity_invalid")
	}
	return nil
}

var dayPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// NormaliseValue checks a value against its attribute and answers it in its stored shape: text
// trimmed, a number as a float, a day as `YYYY-MM-DD`. A file's every value arrives as text, so
// text is read into the attribute's kind.
func NormaliseValue(a Attribute, raw any) (any, error) {
	path := "/attributes/" + a.Key
	if raw == nil {
		return nil, nil
	}
	text, isText := raw.(string)
	if isText {
		text = strings.TrimSpace(text)
		if text == "" {
			return nil, nil
		}
		if len(text) > 2000 || strings.ContainsAny(text, "\n\r") && a.Kind != AttrText {
			return nil, invalid(path, "cmdb.attribute_value_invalid", "key", a.Key)
		}
	}
	switch a.Kind {
	case AttrText:
		if !isText {
			return nil, invalid(path, "cmdb.attribute_value_invalid", "key", a.Key)
		}
		return text, nil
	case AttrNumber:
		switch v := raw.(type) {
		case float64:
			if math.IsNaN(v) || math.IsInf(v, 0) {
				break
			}
			return v, nil
		case int:
			return float64(v), nil
		case string:
			if f, err := strconv.ParseFloat(text, 64); err == nil && !math.IsInf(f, 0) && !math.IsNaN(f) {
				return f, nil
			}
		}
	case AttrBoolean:
		switch v := raw.(type) {
		case bool:
			return v, nil
		case string:
			if b, err := strconv.ParseBool(strings.ToLower(text)); err == nil {
				return b, nil
			}
		}
	case AttrDate:
		if isText && dayPattern.MatchString(text) {
			if _, err := time.Parse(time.DateOnly, text); err == nil {
				return text, nil
			}
		}
	case AttrChoice:
		if isText && slices.Contains(a.Options, text) {
			return text, nil
		}
	case AttrURL:
		if isText {
			if u, err := url.Parse(text); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
				return text, nil
			}
		}
	case AttrGroup, AttrAccount:
		if isText {
			if id, err := shared.ParseID(text); err == nil {
				return id.String(), nil
			}
		}
	}
	return nil, invalid(path, "cmdb.attribute_value_invalid", "key", a.Key)
}

// NormaliseIdentifier is an identifier's value as the index keeps it: trimmed, lower case,
// separators of a MAC address unified.
func NormaliseIdentifier(key string, value any) string {
	var s string
	switch v := value.(type) {
	case string:
		s = v
	case float64:
		s = strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		s = strconv.FormatBool(v)
	default:
		return ""
	}
	s = strings.ToLower(strings.TrimSpace(s))
	if key == "mac_address" {
		s = strings.NewReplacer("-", ":", ".", "").Replace(s)
	}
	if len(s) > 500 {
		s = s[:500]
	}
	return s
}

// Identifiers answers the CI's identifier index: its name and every identifier attribute it
// holds and every attribute a rule names, normalised, keyed by attribute.
func (ci CI) Identifiers(class Resolved) map[string]string {
	out := map[string]string{}
	if n := NormaliseIdentifier(FieldName, ci.Name); n != "" {
		out[FieldName] = n
	}
	inRule := map[string]bool{}
	for _, r := range class.Rules {
		for _, key := range r.Attributes {
			inRule[key] = true
		}
	}
	for _, a := range class.Attributes {
		if !a.Identifier && !inRule[a.Key] {
			continue
		}
		if v := NormaliseIdentifier(a.Key, ci.Attributes[a.Key]); v != "" {
			out[a.Key] = v
		}
	}
	return out
}

// NativeKey is the identifier name a source's own key for a CI is kept under.
func NativeKey(source string) string { return "source:" + source }
