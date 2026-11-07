// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// deadLetterStore is the job table's dead letters, in memory; open names a key already running.
type deadLetterStore struct {
	letters []adminrepo.DeadLetter
	open    map[shared.ID]bool
}

func (s *deadLetterStore) Page(_ context.Context, kind string, before shared.ID, size int) ([]adminrepo.DeadLetter, error) {
	var page []adminrepo.DeadLetter
	for i := len(s.letters) - 1; i >= 0 && len(page) < size; i-- {
		d := s.letters[i]
		if (kind == "" || d.Kind == kind) && (before == "" || d.ID < before) {
			page = append(page, d)
		}
	}
	return page, nil
}

func (s *deadLetterStore) Requeue(_ context.Context, id shared.ID, _ time.Time) (adminrepo.DeadLetter, error) {
	for i, d := range s.letters {
		if d.ID != id {
			continue
		}
		if s.open[id] {
			return adminrepo.DeadLetter{}, shared.ErrConflict.WithDetail("admin.job_already_open")
		}
		s.letters = append(s.letters[:i], s.letters[i+1:]...)
		d.Requeued, d.Attempts, d.FinishedAt = true, 0, time.Time{}
		return d, nil
	}
	return adminrepo.DeadLetter{}, shared.ErrNotFound.WithDetail("admin.dead_letter_not_found")
}

func deadLetters() *deadLetterStore {
	return &deadLetterStore{letters: []adminrepo.DeadLetter{
		{ID: "01936f2a-7c1e-7000-8000-0000000000d1", Kind: "notification.deliver", Attempts: 8, ErrorCode: "dependency.unavailable"},
		{ID: "01936f2a-7c1e-7000-8000-0000000000d2", Kind: "outbox.dispatch", Attempts: 8},
		{ID: "01936f2a-7c1e-7000-8000-0000000000d3", Kind: "notification.deliver", Attempts: 8},
	}, open: map[shared.ID]bool{}}
}

// An operator reads the dead letters newest first, a page at a time and by kind, and nobody outside
// the register reads them at all.
func TestTheOperatorReadsTheDeadLetters(t *testing.T) {
	writer, _, _ := newInstanceWriter(newRegister(operatorID))
	list := ListDeadLetters{Writer: writer, DeadLetters: deadLetters(), Cursors: idCursors{}}

	first, err := list.Execute(t.Context(), operator(), "", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Letters) != 2 || first.Letters[0].ID != "01936f2a-7c1e-7000-8000-0000000000d3" || !first.More {
		t.Fatalf("first page %+v", first)
	}
	second, err := list.Execute(t.Context(), operator(), "", first.Next, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Letters) != 1 || second.More {
		t.Errorf("second page %+v", second)
	}
	byKind, err := list.Execute(t.Context(), operator(), "outbox.dispatch", "", 0)
	if err != nil || len(byKind.Letters) != 1 {
		t.Errorf("by kind %+v, %v", byKind, err)
	}

	outside, _, _ := newInstanceWriter(newRegister(secondOperator))
	if _, err := (ListDeadLetters{Writer: outside, DeadLetters: deadLetters(), Cursors: idCursors{}}).
		Execute(t.Context(), operator(), "", "", 0); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a reader outside the register answered %v", err)
	}
}

// A requeue runs the job again and the journal says so; one whose key is open, and one that is no
// dead letter, are refused and journal nothing.
func TestADeadLetterIsRequeuedAndJournalled(t *testing.T) {
	writer, _, journal := newInstanceWriter(newRegister(operatorID))
	store := deadLetters()
	store.open["01936f2a-7c1e-7000-8000-0000000000d2"] = true
	requeue := RequeueDeadLetter{Writer: writer, DeadLetters: store}

	letter, err := requeue.Execute(t.Context(), operator(), "01936f2a-7c1e-7000-8000-0000000000d1")
	if err != nil {
		t.Fatal(err)
	}
	if out := DeadLetterOutput(letter); out["state"] != "PENDING" || out["finished_at"] != nil {
		t.Errorf("answered %v", out)
	}
	if len(journal.entries) != 1 || journal.entries[0].Action != string(deadLetterRequeuedAction) ||
		journal.entries[0].Details["kind"] != "notification.deliver" {
		t.Fatalf("journal %+v", journal.entries)
	}

	if _, err := requeue.Execute(t.Context(), operator(), "01936f2a-7c1e-7000-8000-0000000000d2"); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("an open key answered %v", err)
	}
	if _, err := requeue.Execute(t.Context(), operator(), "01936f2a-7c1e-7000-8000-0000000000d1"); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a job no longer dead answered %v", err)
	}
	if len(journal.entries) != 1 {
		t.Errorf("a refusal journalled: %+v", journal.entries)
	}
}
