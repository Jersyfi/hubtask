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

// Strip removes the lines naming the addresses, in the caller's transaction where there is one.
// Each is changed as the account that filed it, which wrote the line; where that account may no
// longer, as fallback when it is a person; a refusal is answered after the other lines went. It
// answers how many it changed.
func (s StripIntakeLines) Strip(
	ctx context.Context, tenantID shared.ID, addresses []string, fallback appshared.ActorContext,
) (int, error) {
	if s.Lines == nil || s.Entries == nil || len(addresses) == 0 {
		return 0, nil
	}
	lines, err := s.Lines.IntakeLinesFrom(ctx, addresses)
	if err != nil {
		return 0, err
	}
	changed := 0
	var refusal error
	for _, line := range lines {
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
			return changed, err
		}
		if err != nil {
			// The others still go; the refusal is answered once they have.
			refusal = err
			continue
		}
		changed++
	}
	return changed, refusal
}

// BeforeExpiry removes the lines of the requesters the next REQUESTER batch removes. A line that
// cannot be changed is logged and left, so that one entry nobody may edit any more stops neither
// the expiry nor the rest of the pass.
func (s StripIntakeLines) BeforeExpiry(
	ctx context.Context, tenantID shared.ID, cutoff time.Time, batch int,
) error {
	if s.Lines == nil || s.Entries == nil {
		return nil
	}
	addresses, err := s.Lines.ExpiredAddresses(ctx, cutoff, batch)
	if err != nil || len(addresses) == 0 {
		return err
	}
	for _, address := range addresses {
		if _, err := s.Strip(ctx, tenantID, []string{address}, appshared.ActorContext{}); err != nil {
			if !refused(err) {
				return err
			}
			// The code only: the address is personal data (rule 10).
			slog.WarnContext(ctx, "an expiring requester's intake line could not be removed",
				slog.String("code", shared.AsError(err).Code))
		}
	}
	return nil
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

// refused is an edit the actor may not make, or of something gone meanwhile.
func refused(err error) bool {
	return errors.Is(err, shared.ErrForbidden) || errors.Is(err, shared.ErrNotFound)
}
