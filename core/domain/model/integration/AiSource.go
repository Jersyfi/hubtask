// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"slices"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// AiSources is the instance value `ai.sources`: which sources a workspace may use (ADR-0072 §2).
type AiSources string

const (
	// AiSourcesNone allows no AI at all.
	AiSourcesNone AiSources = "NONE"
	// AiSourcesOffered allows only a model the installation offers.
	AiSourcesOffered AiSources = "OFFERED"
	// AiSourcesOwn allows only the workspace's own model - the product's default, today's
	// behaviour.
	AiSourcesOwn AiSources = "OWN"
	// AiSourcesEither allows both.
	AiSourcesEither AiSources = "EITHER"
)

var aiSourcesValues = []AiSources{AiSourcesNone, AiSourcesOffered, AiSourcesOwn, AiSourcesEither}

// AiSourcesValues is the closed set, in the contract's order.
func AiSourcesValues() []AiSources { return slices.Clone(aiSourcesValues) }

// Valid reports whether the value is one of the defined ones.
func (s AiSources) Valid() bool { return slices.Contains(aiSourcesValues, s) }

// AllowsOwn reports whether the workspace's own model may answer.
func (s AiSources) AllowsOwn() bool { return s == AiSourcesOwn || s == AiSourcesEither }

// AllowsOffered reports whether an offered model may answer.
func (s AiSources) AllowsOffered() bool { return s == AiSourcesOffered || s == AiSourcesEither }

// Within reports whether s allows nothing that above does not: the "stricter" of `ai.sources`
// is a subset, so EITHER contains OFFERED and OWN, and every value contains NONE.
func (s AiSources) Within(above AiSources) bool {
	return (!s.AllowsOwn() || above.AllowsOwn()) && (!s.AllowsOffered() || above.AllowsOffered())
}

func aiSourcesOf(own, offered bool) AiSources {
	switch {
	case own && offered:
		return AiSourcesEither
	case own:
		return AiSourcesOwn
	case offered:
		return AiSourcesOffered
	}
	return AiSourcesNone
}

// AiMinJurisdiction is the instance value `ai.min_jurisdiction`: the least a workspace's own
// model must declare about where it processes content.
type AiMinJurisdiction string

const (
	// AiMinAny admits every declaration the installation admits at all. Whether a third country
	// may be named at all stays the operator's environment decision.
	AiMinAny AiMinJurisdiction = "ANY"
	// AiMinAdequacy admits the EEA, an adequacy decision and the installation's own machine.
	AiMinAdequacy AiMinJurisdiction = "ADEQUACY"
	// AiMinEEA admits the EEA and the installation's own machine.
	AiMinEEA AiMinJurisdiction = "EEA"
	// AiMinSelfHosted admits only a model on the installation's own machine.
	AiMinSelfHosted AiMinJurisdiction = "SELF_HOSTED"
)

// aiMinOrder is also the order of strictness: further right is stricter.
var aiMinOrder = []AiMinJurisdiction{AiMinAny, AiMinAdequacy, AiMinEEA, AiMinSelfHosted}

// AiMinJurisdictions is the closed set, in the contract's order, which is the order of strictness.
func AiMinJurisdictions() []AiMinJurisdiction { return slices.Clone(aiMinOrder) }

// Valid reports whether the value is one of the defined ones.
func (m AiMinJurisdiction) Valid() bool { return slices.Contains(aiMinOrder, m) }

// Admits reports whether a declared jurisdiction meets this minimum.
func (m AiMinJurisdiction) Admits(declared AiJurisdiction) bool {
	return slices.Index(aiMinOrder, m) <= declaredRank(declared)
}

// declaredRank places a declaration on the minimum's scale: a third country meets only ANY.
func declaredRank(declared AiJurisdiction) int {
	switch declared {
	case AiSelfHosted:
		return 3
	case AiEEA:
		return 2
	case AiAdequacy:
		return 1
	}
	return 0
}

// AiSwitch names a key of the instance area `ai`.
type AiSwitch string

const (
	AiSwitchSources         AiSwitch = "sources"
	AiSwitchMinJurisdiction AiSwitch = "min_jurisdiction"
	AiSwitchPersonOptOut    AiSwitch = "person_opt_out"
)

// AiSwitches is every key of the area, in the order a screen draws them.
func AiSwitches() []AiSwitch {
	return []AiSwitch{AiSwitchSources, AiSwitchMinJurisdiction, AiSwitchPersonOptOut}
}

// AiLayer is one level's say about AI: a nil value is a key that level did not decide, and Locks
// the keys it forbids the level below to touch. A workspace's layer carries Sources - its
// narrowing - and its position on people keeping their content out, never a minimum: a workspace
// has no own model below it to hold to one.
type AiLayer struct {
	Sources         *AiSources
	MinJurisdiction *AiMinJurisdiction
	// PersonOptOut is OFFERED at the installation and a plan, or nothing; NOT_OFFERED only at
	// the workspace, and only with Basis.
	PersonOptOut *AiPersonOptOut
	// Basis is the legal basis a workspace named for NOT_OFFERED. Ignored at every other level.
	Basis AiOptOutBasis
	Locks map[AiSwitch]bool
}

// IsZero reports a layer that decided nothing and locked nothing.
func (l AiLayer) IsZero() bool {
	if l.Sources != nil || l.MinJurisdiction != nil || l.PersonOptOut != nil {
		return false
	}
	for _, locked := range l.Locks {
		if locked {
			return false
		}
	}
	return true
}

// AiRule is the rule in force for one workspace, with who fixed what (P-06, P-07).
type AiRule struct {
	// Sources is what the workspace may use now. It never names a source that cannot answer:
	// with nothing offered, OFFERED is not in it (ADR-0072 §7).
	Sources AiSources
	// Above is what the levels above the workspace allow, before the workspace's own narrowing
	// and before dropping what is not offered - what a narrowing is checked against.
	Above           AiSources
	MinJurisdiction AiMinJurisdiction
	// PersonOptOut is whether each person may keep their own content out, and Basis the legal
	// basis the workspace named where it may not.
	PersonOptOut AiPersonOptOut
	Basis        AiOptOutBasis
	Locks        map[AiSwitch]identity.LockOrigin
	Origins      map[AiSwitch]identity.PolicySource
}

// LockOf answers where a key is locked, or LockNone.
func (r AiRule) LockOf(name AiSwitch) identity.LockOrigin { return r.Locks[name] }

// OriginOf answers which level decided a key's value in force.
func (r AiRule) OriginOf(name AiSwitch) identity.PolicySource {
	if origin, held := r.Origins[name]; held {
		return origin
	}
	return identity.SourceDefault
}

// EffectiveAi resolves product default → instance → plan → workspace into the rule in force.
//
// The instance and the plan may set any value - they are the levels a lock comes from. The
// workspace may only narrow, and only where no lock lies, except that narrowing to NONE is always
// its own (ADR-0072 §2: a lock holds a workspace out of a source, never in one). A workspace value
// that is no narrowing of what is above it now is ignored rather than honoured - written before
// the level above moved, the way sign-in's Effective treats a stale row.
func EffectiveAi(instance, plan, workspace AiLayer, offered bool) AiRule {
	rule := AiRule{
		Sources:         AiSourcesOwn,
		MinJurisdiction: AiMinAny,
		PersonOptOut:    AiOptOutOffered,
		Locks:           map[AiSwitch]identity.LockOrigin{},
		Origins:         map[AiSwitch]identity.PolicySource{},
	}

	for _, level := range []struct {
		layer  AiLayer
		origin identity.LockOrigin
		source identity.PolicySource
	}{{instance, identity.LockInstance, identity.SourceInstance}, {plan, identity.LockPlan, identity.SourcePlan}} {
		if level.layer.Sources != nil && level.layer.Sources.Valid() && rule.Locks[AiSwitchSources] == identity.LockNone {
			rule.Sources = *level.layer.Sources
			rule.Origins[AiSwitchSources] = level.source
		}
		if level.layer.MinJurisdiction != nil && level.layer.MinJurisdiction.Valid() &&
			rule.Locks[AiSwitchMinJurisdiction] == identity.LockNone {
			rule.MinJurisdiction = *level.layer.MinJurisdiction
			rule.Origins[AiSwitchMinJurisdiction] = level.source
		}
		// Only OFFERED is honoured above the workspace: a level that cannot name a legal basis
		// cannot make AI part of everybody's work (milestone PH, decision 4).
		if level.layer.PersonOptOut != nil && *level.layer.PersonOptOut == AiOptOutOffered &&
			rule.Locks[AiSwitchPersonOptOut] == identity.LockNone {
			rule.Origins[AiSwitchPersonOptOut] = level.source
		}
		for _, name := range AiSwitches() {
			if level.layer.Locks[name] && rule.Locks[name] == identity.LockNone {
				rule.Locks[name] = level.origin
			}
		}
	}

	rule.Above = rule.Sources
	if narrowed := workspace.Sources; narrowed != nil && narrowed.Valid() && narrowed.Within(rule.Sources) &&
		(rule.Locks[AiSwitchSources] == identity.LockNone || *narrowed == AiSourcesNone) {
		rule.Sources = *narrowed
		rule.Origins[AiSwitchSources] = identity.SourceWorkspace
	}

	// The workspace's NOT_OFFERED stands only with a basis and where nothing is locked; one stored
	// before a lock was placed is ignored, as a stale narrowing is.
	if position := workspace.PersonOptOut; position != nil && rule.Locks[AiSwitchPersonOptOut] == identity.LockNone {
		switch {
		case *position == AiOptOutNotOffered && workspace.Basis.Valid():
			rule.PersonOptOut, rule.Basis = AiOptOutNotOffered, workspace.Basis
			rule.Origins[AiSwitchPersonOptOut] = identity.SourceWorkspace
		case *position == AiOptOutOffered:
			rule.Origins[AiSwitchPersonOptOut] = identity.SourceWorkspace
		}
	}

	if !offered {
		rule.Sources = aiSourcesOf(rule.Sources.AllowsOwn(), false)
	}
	return rule
}

// Narrowed checks a workspace's proposed narrowing against the rule: a locked key refuses
// anything but NONE, and anything wider than what the levels above allow is a loosening.
func (r AiRule) Narrowed(proposed AiSources) error {
	if !proposed.Valid() {
		return fieldError("/sources", "auth.policy_value_invalid")
	}
	if origin := r.LockOf(AiSwitchSources); origin != identity.LockNone && proposed != AiSourcesNone {
		return shared.ErrValidation.
			WithDetail("auth.policy_locked").
			WithParams(map[string]string{"switch": "ai." + string(AiSwitchSources), "origin": string(origin)}).
			WithFields(shared.FieldError{
				Path: "/sources", Code: "auth.policy_locked",
				Params: map[string]string{"origin": string(origin)},
			})
	}
	if !proposed.Within(r.Above) {
		return shared.ErrValidation.
			WithDetail("auth.policy_loosens").
			WithParams(map[string]string{"switch": "ai." + string(AiSwitchSources)}).
			WithFields(shared.FieldError{Path: "/sources", Code: "auth.policy_loosens"})
	}
	return nil
}

// Permits reports whether a workspace may use a source under this rule. NONE always.
func (r AiRule) Permits(source AiSourceKind) bool {
	switch source {
	case AiSourceNone:
		return true
	case AiSourceOwn:
		return r.Sources.AllowsOwn()
	case AiSourceOffered:
		return r.Sources.AllowsOffered()
	}
	return false
}

// AdmitsOwn checks a workspace's own model's declaration against the minimum, naming the minimum
// and who set it so that the sentence can say both (UC-AI-06 check 4).
func (r AiRule) AdmitsOwn(declared AiJurisdiction) error {
	if r.MinJurisdiction.Admits(declared) {
		return nil
	}
	params := map[string]string{
		"minimum": string(r.MinJurisdiction),
		"origin":  string(r.OriginOf(AiSwitchMinJurisdiction)),
	}
	return shared.ErrValidation.
		WithDetail("ai.jurisdiction_below_minimum").
		WithParams(params).
		WithFields(shared.FieldError{Path: "/jurisdiction", Code: "ai.jurisdiction_below_minimum", Params: params})
}

// SourceNotAllowed is the refusal when the source a workspace chose is not one the rule allows -
// at the choice, and at every call once the rule has narrowed past it (ADR-0072 §5).
func SourceNotAllowed() error {
	return shared.ErrForbidden.WithDetail("ai.source_not_allowed")
}

// AiSourceKind is which source a workspace chose to answer its AI requests.
type AiSourceKind string

const (
	AiSourceOwn     AiSourceKind = "OWN"
	AiSourceOffered AiSourceKind = "OFFERED"
	AiSourceNone    AiSourceKind = "NONE"
)

var aiSourceKinds = []AiSourceKind{AiSourceOwn, AiSourceOffered, AiSourceNone}

// AiSourceKinds is the closed set, in the contract's order.
func AiSourceKinds() []AiSourceKind { return slices.Clone(aiSourceKinds) }

// Valid reports whether the kind is one of the defined ones.
func (k AiSourceKind) Valid() bool { return slices.Contains(aiSourceKinds, k) }

// AiChoice is a workspace's choice of source. Choosing an offered model is the workspace's consent
// to send its content there (ADR-0072 §3); choosing its own or none withdraws that consent. The
// own model's consent stays on its own configuration.
//
// No choice stored is OWN: today's behaviour, before anything was offered.
type AiChoice struct {
	TenantID shared.ID
	Source   AiSourceKind
	// OfferedProviderID is the offered model chosen; zero for any other source, and zero after
	// the operator withdrew the model chosen - then the choice answers nothing until made again.
	OfferedProviderID shared.ID
	UpdatedAt         time.Time
	Version           int
}

// DefaultAiChoice is the choice of a workspace that never made one.
func DefaultAiChoice(tenantID shared.ID) AiChoice {
	return AiChoice{TenantID: tenantID, Source: AiSourceOwn}
}

// NewAiChoice validates a choice. Whether the rule allows it is the rule's question; this one is
// only whether the choice is whole.
func NewAiChoice(tenantID shared.ID, source AiSourceKind, offered shared.ID, now time.Time) (AiChoice, error) {
	if tenantID.IsZero() || now.IsZero() {
		return AiChoice{}, shared.ErrInternal.WithDetail("ai.provider_incomplete")
	}
	if !source.Valid() {
		return AiChoice{}, fieldError("/source", "ai.source_unknown")
	}
	switch {
	case source == AiSourceOffered && offered.IsZero():
		return AiChoice{}, fieldError("/offered_model_id", "ai.offered_model_unknown")
	case source != AiSourceOffered && !offered.IsZero():
		return AiChoice{}, fieldError("/offered_model_id", "ai.offered_model_without_offered_source")
	}
	return AiChoice{TenantID: tenantID, Source: source, OfferedProviderID: offered, UpdatedAt: now.UTC(), Version: 1}, nil
}

// OfferedEmbeddingSpace is the name an offered model's vectors are stored under. Vectors are
// comparable only within one model, and a workspace's own model and an offered one may carry the
// same name and be different models; qualifying the offered one keeps the two spaces apart, while
// the own model keeps its bare name so that nothing is re-embedded by an upgrade.
func OfferedEmbeddingSpace(providerID shared.ID, model string) string {
	if model == "" {
		return ""
	}
	return "offered:" + providerID.String() + "/" + model
}
