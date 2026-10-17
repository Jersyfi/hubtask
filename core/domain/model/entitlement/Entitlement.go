// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package entitlement holds the features a plan may leave out of a workspace (multi-tenancy.md
// §4.2), and what a workspace makes of the set its plan leaves out.
//
// Its own package because both the plan, which decides the set, and the workspace, which carries
// it to the sign-in readers, need it - and the plan already depends on identity.
package entitlement

import (
	"slices"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Feature is one of the closed set a plan may leave out. The set is closed on purpose: export,
// deletion, access requests, language, time zone, accessibility and every door are no entry of it,
// so no plan can switch them off (UC-INS-13 check 5, P-08).
type Feature string

const (
	AutomationRules    Feature = "AUTOMATION_RULES"
	Webhooks           Feature = "WEBHOOKS"
	OwnSignInProviders Feature = "OWN_SIGN_IN_PROVIDERS"
)

var features = []Feature{AutomationRules, Webhooks, OwnSignInProviders}

// Features is the closed set, in the order a screen draws it.
func Features() []Feature { return slices.Clone(features) }

// Valid reports whether the feature is one a plan can leave out.
func (f Feature) Valid() bool { return slices.Contains(features, f) }

// NotIncludedCode is the refusal of using a feature the workspace's plan leaves out.
const NotIncludedCode = "plan.not_included"

// LeftOut is what a workspace's plan leaves out. The zero value - no plan, or a plan that leaves
// out nothing - includes every feature, which is a workspace's state without plans.
type LeftOut []Feature

// Includes reports whether the workspace has the feature.
func (l LeftOut) Includes(feature Feature) bool { return !slices.Contains(l, feature) }

// Require refuses a feature the plan leaves out. Forbidden rather than invalid: the request is
// well formed, and what would change the answer is the plan, not the request.
func (l LeftOut) Require(feature Feature) error {
	if l.Includes(feature) {
		return nil
	}
	return NotIncluded(feature)
}

// NotIncluded is the refusal naming the feature.
func NotIncluded(feature Feature) error {
	return shared.ErrForbidden.WithDetail(NotIncludedCode).
		WithParams(map[string]string{"feature": string(feature)})
}

// Ordered is the set in the domain's order, each feature once, so that two reads of one plan
// answer the same list.
func Ordered(held []Feature) LeftOut {
	ordered := make(LeftOut, 0, len(held))
	for _, feature := range features {
		if slices.Contains(held, feature) {
			ordered = append(ordered, feature)
		}
	}
	return ordered
}
