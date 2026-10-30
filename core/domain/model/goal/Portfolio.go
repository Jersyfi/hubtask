// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package goal

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// The bounds of portfolios.
const (
	MaxPortfolioItems  = 200
	MaxSummaryLength   = 10000
	MaxPortfolioNested = 6
)

// Portfolio is a set of hubs and collections - projects - kept together with an owner, a status
// its owner gives it at an update, and an optional parent portfolio.
type Portfolio struct {
	ID          shared.ID
	ParentID    shared.ID
	Name        string
	Description string
	OwnerID     shared.ID
	Status      Status
	Items       []PortfolioItem
	Version     int
}

// PortfolioItem is a project of a portfolio: a hub or a collection, with the dates the portfolio
// plans it for and its own health as the portfolio judges it.
type PortfolioItem struct {
	ContainerID shared.ID
	StartOn     *Day
	EndOn       *Day
	Status      Status
}

// CheckPortfolio checks a portfolio's fields and its projects: each once, dates in order.
func CheckPortfolio(p Portfolio, normalizer text.Normalizer) (Portfolio, error) {
	name, err := title(p.Name, normalizer, "/name", "portfolios.name")
	if err != nil {
		return Portfolio{}, err
	}
	p.Name = name
	p.Description = strings.TrimSpace(p.Description)
	if normalizer != nil {
		p.Description = normalizer.NFC(p.Description)
	}
	if utf8.RuneCountInString(p.Description) > MaxDescriptionLength {
		return Portfolio{}, invalid("/description", "portfolios.description_too_long")
	}
	if p.Status == "" {
		p.Status = StatusNone
	}
	if p.Status != StatusNone && !p.Status.Health() {
		return Portfolio{}, invalid("/status", "portfolios.status_invalid")
	}
	if len(p.Items) > MaxPortfolioItems {
		return Portfolio{}, invalid("/items", "portfolios.too_many_items")
	}
	seen := map[shared.ID]bool{}
	for i, item := range p.Items {
		at := "/items/" + itoa(i)
		if item.ContainerID.IsZero() || seen[item.ContainerID] {
			return Portfolio{}, invalid(at+"/container_id", "portfolios.item_duplicate")
		}
		seen[item.ContainerID] = true
		if item.StartOn != nil && item.EndOn != nil && *item.EndOn < *item.StartOn {
			return Portfolio{}, invalid(at+"/end_on", "portfolios.range_reversed")
		}
		if item.Status == "" {
			p.Items[i].Status = StatusNone
		} else if item.Status != StatusNone && !item.Status.Health() {
			return Portfolio{}, invalid(at+"/status", "portfolios.status_invalid")
		}
	}
	return p, nil
}

// PortfolioNesting checks that a portfolio may sit under a parent: no cycle and no deeper than
// MaxPortfolioNested. chain is the parent and its parents, nearest first.
func PortfolioNesting(portfolio shared.ID, chain []shared.ID) error {
	for _, id := range chain {
		if id == portfolio {
			return invalid("/parent_id", "portfolios.parent_cycle")
		}
	}
	if len(chain) >= MaxPortfolioNested {
		return invalid("/parent_id", "portfolios.too_deep")
	}
	return nil
}

// StatusUpdate is what a portfolio's owner wrote about how it stands, with the status it gave.
// Updates are history; none changes.
type StatusUpdate struct {
	ID          shared.ID
	PortfolioID shared.ID
	AuthorID    shared.ID
	Status      Status
	Summary     string
	At          time.Time
}

// CheckStatusUpdate checks an update: a judging status and a summary within bounds.
func CheckStatusUpdate(u StatusUpdate, normalizer text.Normalizer) (StatusUpdate, error) {
	if !u.Status.Health() {
		return StatusUpdate{}, invalid("/status", "portfolios.status_invalid")
	}
	u.Summary = strings.TrimSpace(u.Summary)
	if normalizer != nil {
		u.Summary = normalizer.NFC(u.Summary)
	}
	if u.Summary == "" {
		return StatusUpdate{}, invalid("/summary", "portfolio_updates.summary_required")
	}
	if utf8.RuneCountInString(u.Summary) > MaxSummaryLength {
		return StatusUpdate{}, invalid("/summary", "portfolio_updates.summary_too_long")
	}
	return u, nil
}
