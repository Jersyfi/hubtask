// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"slices"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Sufferers answers, for every CI, the CIs that suffer when it fails - one step of the impact
// graph, read through each relation type's impact.
func Sufferers(relations []Relation, types RelationTypes) map[shared.ID][]shared.ID {
	out := map[shared.ID][]shared.ID{}
	for _, r := range relations {
		t, ok := types.Find(r.Type)
		if !ok {
			continue
		}
		if t.Impact == ImpactSource || t.Impact == ImpactBoth {
			out[r.TargetCI] = append(out[r.TargetCI], r.SourceCI)
		}
		if t.Impact == ImpactTarget || t.Impact == ImpactBoth {
			out[r.SourceCI] = append(out[r.SourceCI], r.TargetCI)
		}
	}
	return out
}

// EffectiveProtection answers every CI's protection need with what it inherits: the maximum of
// its own and that of everything that suffers when it fails, transitively (BSI 200-2, the maximum
// principle). A dimension its owner fixed keeps the own value - the distribution effect, with a
// reason on record.
func EffectiveProtection(cis []CI, relations []Relation, types RelationTypes) map[shared.ID]Protection {
	own := map[shared.ID]OwnProtection{}
	effective := map[shared.ID]Protection{}
	for _, ci := range cis {
		own[ci.ID] = ci.Compliance.Protection
		effective[ci.ID] = ci.Compliance.Protection.Protection
	}
	sufferers := Sufferers(relations, types)
	for range len(cis) + 1 {
		changed := false
		for _, ci := range cis {
			next := effective[ci.ID]
			for _, s := range sufferers[ci.ID] {
				if p, ok := effective[s]; ok {
					next = next.Max(p)
				}
			}
			fixed := own[ci.ID].Fixed
			if slices.Contains(fixed, "c") {
				next.C = own[ci.ID].C
			}
			if slices.Contains(fixed, "i") {
				next.I = own[ci.ID].I
			}
			if slices.Contains(fixed, "a") {
				next.A = own[ci.ID].A
			}
			if next != effective[ci.ID] {
				effective[ci.ID], changed = next, true
			}
		}
		if !changed {
			break
		}
	}
	return effective
}

// Problem names one CI health found wanting and why, by codes.
type Problem struct {
	CI    shared.ID
	Class string
	Kinds []string
	// Missing are the required attributes it lacks.
	Missing []string
}

// The kinds of problem health reports.
const (
	ProblemIncomplete    = "INCOMPLETE"
	ProblemStale         = "STALE"
	ProblemOrphan        = "ORPHAN"
	ProblemDuplicate     = "DUPLICATE"
	ProblemNoOwner       = "NO_OWNER"
	ProblemReviewOverdue = "REVIEW_OVERDUE"
)

// ClassHealth is one class's figures.
type ClassHealth struct {
	Class        string
	Total        int
	Completeness float64
	Stale        int
	Orphans      int
	Duplicates   int
}

// Health is a hub's inventory judged (research itsm-cmdb-ai H1-H3): completeness of required
// and recommended attributes, staleness, orphans, duplicates, CIs without an owner and reviews
// overdue.
type Health struct {
	Total         int
	Completeness  float64
	Correctness   float64
	Stale         int
	Orphans       int
	Duplicates    int
	WithoutOwner  int
	ReviewOverdue int
	Classes       []ClassHealth
	Problems      []Problem
}

// MaxProblems bounds the problems one answer lists.
const MaxProblems = 500

// Judge answers a hub's health. Only active CIs count; duplicates are the CIs named by an open
// duplicate to merge.
func Judge(cis []CI, relations []Relation, classes Classes, duplicates map[shared.ID]bool, now time.Time) Health {
	type link struct {
		typ      string
		outgoing bool
	}
	links := map[shared.ID][]link{}
	for _, r := range relations {
		links[r.SourceCI] = append(links[r.SourceCI], link{r.Type, true})
		links[r.TargetCI] = append(links[r.TargetCI], link{r.Type, false})
	}
	per := map[string]*ClassHealth{}
	var h Health
	var filled, wanted float64
	var wrong int
	for _, ci := range cis {
		if !ci.Stage.Active() {
			continue
		}
		class := classes.Resolve(ci.Class)
		c := per[ci.Class]
		if c == nil {
			c = &ClassHealth{Class: ci.Class}
			per[ci.Class] = c
		}
		c.Total++
		h.Total++
		problem := Problem{CI: ci.ID, Class: ci.Class}
		var ciFilled, ciWanted float64
		for _, a := range class.Attributes {
			weight := map[Need]float64{NeedRequired: 2, NeedRecommended: 1}[a.Need]
			if weight == 0 {
				continue
			}
			ciWanted += weight
			if _, ok := ci.Attributes[a.Key]; ok {
				ciFilled += weight
			} else if a.Need == NeedRequired {
				problem.Missing = append(problem.Missing, a.Key)
			}
		}
		ciWanted += 2
		if !ci.Compliance.OwnerAccount.IsZero() || !ci.Compliance.OwnerGroup.IsZero() {
			ciFilled += 2
		} else {
			problem.Kinds = append(problem.Kinds, ProblemNoOwner)
			h.WithoutOwner++
		}
		if len(problem.Missing) > 0 {
			problem.Kinds = append(problem.Kinds, ProblemIncomplete)
		}
		filled += ciFilled
		wanted += ciWanted
		c.Completeness += ciFilled / ciWanted
		stale := ci.LastSeen == nil || now.Sub(*ci.LastSeen) > time.Duration(class.StalenessDays)*24*time.Hour
		if stale {
			problem.Kinds = append(problem.Kinds, ProblemStale)
			c.Stale++
			h.Stale++
		}
		for _, req := range class.Required {
			if !slices.ContainsFunc(links[ci.ID], func(l link) bool {
				return l.outgoing == req.Outgoing && slices.Contains(req.Types, l.typ)
			}) {
				problem.Kinds = append(problem.Kinds, ProblemOrphan)
				c.Orphans++
				h.Orphans++
				break
			}
		}
		if duplicates[ci.ID] {
			problem.Kinds = append(problem.Kinds, ProblemDuplicate)
			c.Duplicates++
			h.Duplicates++
		}
		if ci.Compliance.ReviewDue != nil && now.After(ci.Compliance.ReviewDue.Add(24*time.Hour)) {
			problem.Kinds = append(problem.Kinds, ProblemReviewOverdue)
			h.ReviewOverdue++
		}
		if stale || duplicates[ci.ID] || slices.Contains(problem.Kinds, ProblemOrphan) {
			wrong++
		}
		if len(problem.Kinds) > 0 && len(h.Problems) < MaxProblems {
			h.Problems = append(h.Problems, problem)
		}
	}
	h.Completeness, h.Correctness = 1, 1
	if wanted > 0 {
		h.Completeness = filled / wanted
	}
	if h.Total > 0 {
		h.Correctness = 1 - float64(wrong)/float64(h.Total)
	}
	for _, c := range per {
		c.Completeness /= float64(c.Total)
		h.Classes = append(h.Classes, *c)
	}
	slices.SortFunc(h.Classes, func(a, b ClassHealth) int { return b.Total - a.Total })
	return h
}
