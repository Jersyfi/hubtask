// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The use cases the lines are removed through: the ordinary ones, so that the change log, the
// devices, the search and the audit trail follow as they do for any edit (third answers 2).
const (
	updateWorkItemName = "UpdateWorkItem"
	editCommentName    = "EditComment"
	deleteCommentName  = "DeleteComment"
	itemsWrite         = "items:write"
	commentsWrite      = "comments:write"
)

// Invoker runs a use case of the catalogue by name.
type Invoker interface {
	Invoke(ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error)
}

// StripIntakeLines removes the mail intake's "From: name <address>" line from the entries and
// comments an account filed on a requester's behalf, when the requester is erased or expires.
type StripIntakeLines struct {
	Lines   repository.IntakeLines
	Entries Invoker
}

// IntakeLinePage is how many lines one read answers. A requester who wrote for years has more
// lines than one read holds; the next read starts after the last line of the page.
const IntakeLinePage = 500

// StripStep is what one page of Strip did: the lines it changed, the cursor after the page, whether
// a page may follow, and the last refusal among its lines.
type StripStep struct {
	Changed int
	Next    repository.IntakeCursor
	More    bool
	Refusal error
}

// Strip removes every line naming the addresses, page by page in the caller's transaction where
// there is one. It answers how many it changed and, after the others went, a refusal.
func (s StripIntakeLines) Strip(
	ctx context.Context, tenantID shared.ID, addresses []string, fallback appshared.ActorContext,
) (int, error) {
	changed := 0
	var refusal error
	step := StripStep{More: true}
	for step.More {
		var err error
		step, err = s.StripPage(ctx, tenantID, addresses, fallback, step.Next)
		changed += step.Changed
		if err != nil {
			return changed, err
		}
		if step.Refusal != nil {
			refusal = step.Refusal
		}
	}
	return changed, refusal
}

// StripPage removes the lines of one page after the cursor. Each is changed as the account that
// filed it, which wrote the line; where that account may no longer, as fallback when it is a
// person. A refused line is passed over: the cursor moves on, so it never blocks the lines behind
// it, and the refusal is answered in the step.
func (s StripIntakeLines) StripPage(
	ctx context.Context, tenantID shared.ID, addresses []string, fallback appshared.ActorContext,
	after repository.IntakeCursor,
) (StripStep, error) {
	step := StripStep{Next: after}
	if s.Lines == nil || s.Entries == nil || len(addresses) == 0 {
		return step, nil
	}
	lines, err := s.Lines.IntakeLinesFrom(ctx, addresses, after, IntakeLinePage)
	if err != nil {
		return step, err
	}
	step.More = len(lines) == IntakeLinePage
	for _, line := range lines {
		step.Next = line.After()
		text, stripped := stripAny(line.Text, addresses)
		if !stripped {
			continue
		}
		name, in := lineChange(line, text)
		filer := appshared.ActorContext{
			Kind: appshared.ActorAutomation, TenantID: tenantID, AccountID: line.WrittenBy,
			Scopes: []string{itemsWrite, commentsWrite},
		}
		_, err := s.Entries.Invoke(ctx, name, filer, in)
		if err != nil && fallback.Kind == appshared.ActorUser && refused(err) {
			_, err = s.Entries.Invoke(ctx, name, fallback, in)
		}
		if err != nil && !refused(err) {
			return step, err
		}
		if err != nil {
			step.Refusal = err
			continue
		}
		step.Changed++
	}
	return step, nil
}

// AllIntakeLines answers every line whose first line names one of the addresses, read page by page.
func AllIntakeLines(ctx context.Context, lines repository.IntakeLines, addresses []string) ([]repository.IntakeLine, error) {
	var all []repository.IntakeLine
	var after repository.IntakeCursor
	for {
		page, err := lines.IntakeLinesFrom(ctx, addresses, after, IntakeLinePage)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if len(page) < IntakeLinePage {
			return all, nil
		}
		after = page[len(page)-1].After()
	}
}

// anyNamed answers whether a line still names one of the addresses exactly.
func anyNamed(ctx context.Context, lines repository.IntakeLines, addresses []string) (bool, error) {
	var after repository.IntakeCursor
	for {
		page, err := lines.IntakeLinesFrom(ctx, addresses, after, IntakeLinePage)
		if err != nil {
			return false, err
		}
		for _, line := range page {
			if _, named := stripAny(line.Text, addresses); named {
				return true, nil
			}
		}
		if len(page) < IntakeLinePage {
			return false, nil
		}
		after = page[len(page)-1].After()
	}
}

// expiryReadPages bounds how many pages of kept requesters one expiry reads past: the retention
// read budget of ten batches (data-retention.md §5).
const expiryReadPages = 10

// BeforeExpiry removes the lines of up to a batch of expired requesters, oldest first, and answers
// the addresses whose lines are all gone - only those requesters may go - and how many it kept. A
// line that cannot be changed - nobody may edit it, or its collection is archived or trashed -
// stops neither the expiry nor the rest of the pass; its requester stays, so the record of where
// the address still is outlives no line, and the next pass tries again. The expiry reads past kept
// requesters, so a batch of them does not hide the ones behind it.
func (s StripIntakeLines) BeforeExpiry(
	ctx context.Context, tenantID shared.ID, cutoff time.Time, batch int,
) ([]string, int, error) {
	if s.Lines == nil || s.Entries == nil || batch <= 0 {
		return nil, 0, nil
	}
	cleared := make([]string, 0, batch)
	kept := 0
	for page := 0; page < expiryReadPages && len(cleared) < batch; page++ {
		addresses, err := s.Lines.ExpiredAddresses(ctx, cutoff, page*batch, batch)
		if err != nil {
			return nil, 0, err
		}
		for _, address := range addresses {
			if len(cleared) == batch {
				break
			}
			if _, err := s.Strip(ctx, tenantID, []string{address}, appshared.ActorContext{}); err != nil {
				if !refused(err) {
					return nil, 0, err
				}
				kept++
				continue
			}
			cleared = append(cleared, address)
		}
		if len(addresses) < batch {
			break
		}
	}
	if kept > 0 {
		// A count only: the addresses are personal data (rule 10).
		slog.WarnContext(ctx, "expiring requesters kept while an intake line naming them cannot be changed",
			slog.Int("kept", kept))
	}
	return cleared, kept, nil
}

func stripAny(text string, addresses []string) (string, bool) {
	for _, address := range addresses {
		if out, stripped := domain.StripIntakeLine(text, address); stripped {
			return out, true
		}
	}
	return text, false
}

// lineChange is the edit that removes the line: the notes rewritten, the comment rewritten, or the
// comment deleted where nothing but the line was in it.
func lineChange(line repository.IntakeLine, text string) (string, usecase.Input) {
	if !line.Comment {
		return updateWorkItemName, usecase.Input{"item_id": line.ItemID.String(), "notes": text}
	}
	in := usecase.Input{"item_id": line.ItemID.String(), "comment_id": line.ID.String()}
	if strings.TrimSpace(text) == "" {
		return deleteCommentName, in
	}
	in["body"] = text
	return editCommentName, in
}

// refused is an edit that cannot be made now: the actor may not make it, its subject is gone, or
// its collection is archived or trashed and so read-only (I-C3). The line stays and is retried.
func refused(err error) bool {
	return errors.Is(err, shared.ErrForbidden) || errors.Is(err, shared.ErrNotFound) ||
		errors.Is(err, shared.ErrConflict)
}
