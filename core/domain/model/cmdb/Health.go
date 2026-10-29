// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"slices"
	"strings"
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
	// StaleRelations counts the relations a program reported that it has not confirmed within the
	// staleness period of their source CI's class (R10); a person's own never go stale.
	StaleRelations int
	Services       []ServiceHealth
	Sources        []SourceConflicts
}

// ServiceHealth is a service's score (H7): the share of the active CIs it rests on - itself and
// everything whose failure it suffers, transitively - that health finds wanting in nothing.
type ServiceHealth struct {
	Service shared.ID
	Class   string
	Name    string
	CIs     int
	Wanting int
	Score   float64
}

// SourceConflicts is one source's conflict rate over the runs read (H5): of the attributes it
// reported with a value other than the stored one, the share a better source kept.
type SourceConflicts struct {
	Source   string
	Reported int
	Kept     int
	Rate     float64
}

// MaxServices bounds the services one answer scores.
const MaxServices = 200

// Conflicts answers each source's conflict rate from its runs' decisions, by source key.
func Conflicts(runs []Run) []SourceConflicts {
	per := map[string]*SourceConflicts{}
	for _, run := range runs {
		c := per[run.Source]
		if c == nil {
			c = &SourceConflicts{Source: run.Source}
			per[run.Source] = c
		}
		for _, d := range run.Decisions {
			c.Reported += len(d.Taken) + len(d.Kept)
			c.Kept += len(d.Kept)
		}
	}
	out := make([]SourceConflicts, 0, len(per))
	for _, c := range per {
		if c.Reported > 0 {
			c.Rate = float64(c.Kept) / float64(c.Reported)
		}
		out = append(out, *c)
	}
	slices.SortFunc(out, func(a, b SourceConflicts) int {
		if a.Rate != b.Rate {
			if a.Rate > b.Rate {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Source, b.Source)
	})
	return out
}

// supports answers, for every CI, the CIs whose failure it suffers - Sufferers read backwards.
func supports(relations []Relation, types RelationTypes) map[shared.ID][]shared.ID {
	out := map[shared.ID][]shared.ID{}
	for ci, sufferers := range Sufferers(relations, types) {
		for _, s := range sufferers {
			out[s] = append(out[s], ci)
		}
	}
	return out
}

// judgeServices scores every active service by the CIs it rests on.
func judgeServices(cis []CI, relations []Relation, classes Classes, types RelationTypes, wanting map[shared.ID]bool) []ServiceHealth {
	byID := map[shared.ID]CI{}
	for _, ci := range cis {
		byID[ci.ID] = ci
	}
	under := supports(relations, types)
	var out []ServiceHealth
	for _, ci := range cis {
		if !ci.Stage.Active() || !classes.IsA(ci.Class, "service") || len(out) >= MaxServices {
			continue
		}
		seen := map[shared.ID]bool{ci.ID: true}
		queue := []shared.ID{ci.ID}
		for len(queue) > 0 {
			at := queue[0]
			queue = queue[1:]
			for _, next := range under[at] {
				if !seen[next] {
					seen[next] = true
					queue = append(queue, next)
				}
			}
		}
		h := ServiceHealth{Service: ci.ID, Class: ci.Class, Name: ci.Name}
		for id := range seen {
			if other, ok := byID[id]; ok && other.Stage.Active() {
				h.CIs++
				if wanting[id] {
					h.Wanting++
				}
			}
		}
		h.Score = 1
		if h.CIs > 0 {
			h.Score = 1 - float64(h.Wanting)/float64(h.CIs)
		}
		out = append(out, h)
	}
	slices.SortFunc(out, func(a, b ServiceHealth) int {
		if a.Score != b.Score {
			if a.Score < b.Score {
				return -1
			}
			return 1
		}
		return strings.Compare(string(a.Service), string(b.Service))
	})
	return out
}

// staleRelations counts the relations a program reported and has not confirmed since its source
// CI's class went stale.
func staleRelations(cis []CI, relations []Relation, classes Classes, now time.Time) int {
	classOf := map[shared.ID]string{}
	for _, ci := range cis {
		classOf[ci.ID] = ci.Class
	}
	n := 0
	for _, r := range relations {
		if r.DataSource == ManualSource {
			continue
		}
		class, ok := classOf[r.SourceCI]
		if !ok {
			continue
		}
		if now.Sub(r.LastSeen) > time.Duration(classes.Resolve(class).StalenessDays)*24*time.Hour {
			n++
		}
	}
	return n
}

// MaxProblems bounds the problems one answer lists.
const MaxProblems = 500

// Judge answers a hub's health. Only active CIs count; duplicates are the CIs named by an open
// duplicate to merge.
func Judge(cis []CI, relations []Relation, classes Classes, types RelationTypes, duplicates map[shared.ID]bool, runs []Run, now time.Time) Health {
	wanting := map[shared.ID]bool{}
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
		if len(problem.Kinds) > 0 {
			wanting[ci.ID] = true
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
	h.StaleRelations = staleRelations(cis, relations, classes, now)
	h.Services = judgeServices(cis, relations, classes, types, wanting)
	h.Sources = Conflicts(runs)
	return h
}
