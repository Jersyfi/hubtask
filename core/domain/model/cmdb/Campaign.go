// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"slices"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// MaxCampaignCIs bounds the CIs one certification campaign asks about.
const MaxCampaignCIs = 5000

// DefaultReviewMonths is how far a confirmed CI's next review moves when a campaign names no
// period: NIS2's "at least annually".
const DefaultReviewMonths = 12

// Verdict is what an owner said of a CI in a campaign.
type Verdict string

const (
	// VerdictPending is not yet answered.
	VerdictPending Verdict = "PENDING"
	// VerdictConfirmed says the CI is as recorded; its next review moves on.
	VerdictConfirmed Verdict = "CONFIRMED"
	// VerdictDisputed says the record is wrong or the object is gone; the CI is marked for review.
	VerdictDisputed Verdict = "DISPUTED"
)

// Campaign asks the owners of a hub's CIs to attest them by a day (research H4): the CIs of the
// classes named, active, and - when DueBy is set - whose review falls due by then or was never
// set. The set is fixed when it starts, so its score does not move with the inventory.
type Campaign struct {
	ID           shared.ID
	TenantID     shared.ID
	HubID        shared.ID
	Name         string
	Classes      []string
	DueBy        *time.Time
	DueOn        time.Time
	ReviewMonths int
	StartedBy    shared.ID
	StartedAt    time.Time
	ClosedAt     *time.Time
	ClosedBy     shared.ID
	Version      int
}

// Attestation is one CI's question in a campaign, and its answer.
type Attestation struct {
	CampaignID shared.ID
	CIID       shared.ID
	// Owner is the CI's owning person when the campaign started; zero when it had none, and then
	// anybody who may write the hub answers.
	Owner      shared.ID
	Verdict    Verdict
	AttestedBy shared.ID
	AttestedAt *time.Time
}

// CampaignInput is what a person starts a campaign with.
type CampaignInput struct {
	Name         string
	Classes      []string
	DueBy        *time.Time
	DueOn        time.Time
	ReviewMonths int
}

// NewCampaign checks the input against the workspace's classes.
func NewCampaign(id, tenant, hub, by shared.ID, in CampaignInput, classes Classes, now time.Time) (Campaign, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || len([]rune(name)) > MaxNameLength || strings.ContainsAny(name, "\n\r") {
		return Campaign{}, invalid("/name", "cmdb.campaign_name_invalid")
	}
	for _, c := range in.Classes {
		if _, ok := classes.Find(c); !ok {
			return Campaign{}, invalid("/classes", "cmdb.class_unknown", c)
		}
	}
	if !in.DueOn.After(now) {
		return Campaign{}, invalid("/due_on", "cmdb.campaign_due_past")
	}
	months := in.ReviewMonths
	if months == 0 {
		months = DefaultReviewMonths
	}
	if months < 1 || months > 60 {
		return Campaign{}, invalid("/review_months", "cmdb.campaign_period_invalid")
	}
	cs := slices.Clone(in.Classes)
	slices.Sort(cs)
	return Campaign{
		ID: id, TenantID: tenant, HubID: hub, Name: name, Classes: slices.Compact(cs), DueBy: in.DueBy,
		DueOn: in.DueOn, ReviewMonths: months, StartedBy: by, StartedAt: now, Version: 1,
	}, nil
}

// InScope says whether a CI is asked about: active, of a named class or below it (all when none is
// named), and with a review falling due by DueBy when that is set.
func (c Campaign) InScope(ci CI, classes Classes) bool {
	if !ci.Stage.Active() || ci.Status == StatusMerged {
		return false
	}
	if len(c.Classes) > 0 && !slices.ContainsFunc(c.Classes, func(k string) bool { return classes.IsA(ci.Class, k) }) {
		return false
	}
	if c.DueBy != nil && ci.Compliance.ReviewDue != nil && ci.Compliance.ReviewDue.After(*c.DueBy) {
		return false
	}
	return true
}

// Closed says whether answers are no longer taken.
func (c Campaign) Closed() bool { return c.ClosedAt != nil }

// Answer records a verdict. A closed campaign takes none; a pending answer is not one.
func (c Campaign) Answer(a Attestation, v Verdict, by shared.ID, now time.Time) (Attestation, error) {
	if c.Closed() {
		return a, shared.ErrConflict.WithDetail("cmdb.campaign_closed")
	}
	if v != VerdictConfirmed && v != VerdictDisputed {
		return a, invalid("/verdict", "cmdb.verdict_unknown")
	}
	a.Verdict, a.AttestedBy, a.AttestedAt = v, by, &now
	return a, nil
}

// NextReview is the day a confirmed CI is reviewed again.
func (c Campaign) NextReview(now time.Time) time.Time {
	y, m, d := now.UTC().Date()
	return time.Date(y, m+time.Month(c.ReviewMonths), d, 0, 0, 0, 0, time.UTC)
}

// Score is a campaign's standing: how many were asked, answered each way, and the share answered.
type Score struct {
	Total     int
	Confirmed int
	Disputed  int
	Pending   int
	// Overdue is the pending ones once the due day passed.
	Overdue int
	// Rate is the share answered, 0 to 1; 1 when nothing was asked.
	Rate float64
}

// Tally scores the answers at a moment.
func (c Campaign) Tally(as []Attestation, now time.Time) Score {
	s := Score{Total: len(as)}
	for _, a := range as {
		switch a.Verdict {
		case VerdictConfirmed:
			s.Confirmed++
		case VerdictDisputed:
			s.Disputed++
		default:
			s.Pending++
		}
	}
	if now.After(c.DueOn) {
		s.Overdue = s.Pending
	}
	s.Rate = 1
	if s.Total > 0 {
		s.Rate = float64(s.Confirmed+s.Disputed) / float64(s.Total)
	}
	return s
}
