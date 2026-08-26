// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The ports carry no logic, so what is held in place here is their shape: that one fake can
// implement all four, which is what the use case tests depend on, and the two properties a caller
// is entitled to rely on.

type double struct {
	stored    map[shared.ID]domain.Request
	consents  []domain.Consent
	statuses  map[shared.ID]string
	pseudonym map[shared.ID]string
}

func newDouble() *double {
	return &double{
		stored: map[shared.ID]domain.Request{}, statuses: map[shared.ID]string{},
		pseudonym: map[shared.ID]string{},
	}
}

func (d *double) Insert(_ context.Context, request domain.Request) error {
	d.stored[request.ID] = request
	return nil
}

func (d *double) Find(_ context.Context, id shared.ID) (domain.Request, error) {
	request, found := d.stored[id]
	if !found {
		return domain.Request{}, shared.ErrNotFound.WithDetail(domain.CodeRequestNotFound)
	}
	return request, nil
}

func (d *double) Save(_ context.Context, request domain.Request) (bool, error) {
	if _, found := d.stored[request.ID]; !found {
		return false, nil
	}
	d.stored[request.ID] = request
	return true, nil
}

func (d *double) List(_ context.Context, filter Filter) (Page, error) {
	var out []domain.Request
	for _, request := range d.stored {
		if !filter.IncludeClosed && request.Status.Closed() {
			continue
		}
		out = append(out, request)
	}
	return Page{Requests: out}, nil
}

func (d *double) Deadlines(_ context.Context, now time.Time) (Deadlines, error) {
	var deadlines Deadlines
	for _, request := range d.stored {
		if request.Status.Closed() {
			continue
		}
		deadlines.Open++
		if request.Overdue(now) {
			deadlines.Overdue++
		}
		if deadlines.NextDueAt.IsZero() || request.DueAt.Before(deadlines.NextDueAt) {
			deadlines.NextDueAt = request.DueAt
		}
	}
	return deadlines, nil
}

func (d *double) Withdraw(_ context.Context, _ shared.ID, purpose string, at time.Time) (int, error) {
	ended := 0
	for i, consent := range d.consents {
		if consent.Purpose == purpose && consent.InForce() {
			d.consents[i] = consent.Withdraw(at)
			ended++
		}
	}
	return ended, nil
}

func (d *double) Record(_ context.Context, consent domain.Consent) error {
	d.consents = append(d.consents, consent)
	return nil
}

func (d *double) Latest(_ context.Context, _ shared.ID, purpose string) (domain.Consent, error) {
	for i := len(d.consents) - 1; i >= 0; i-- {
		if d.consents[i].Purpose == purpose {
			return d.consents[i], nil
		}
	}
	return domain.Consent{}, shared.ErrNotFound.WithDetail(domain.CodeConsentNotFound)
}

func (d *double) SetStatus(_ context.Context, id shared.ID, status string, _ time.Time) (bool, error) {
	d.statuses[id] = status
	return true, nil
}

func (d *double) Tenants(context.Context, string) ([]shared.ID, error) { return nil, nil }

func (d *double) Assign(_ context.Context, actorID shared.ID, pseudonym, _ string, _ time.Time) error {
	if _, already := d.pseudonym[actorID]; already {
		// Idempotent on the actor, which is the property the port promises: an erasure that is
		// retried records the same pseudonym rather than a second one.
		return nil
	}
	d.pseudonym[actorID] = pseudonym
	return nil
}

func (d *double) For(_ context.Context, actorIDs []shared.ID) (map[shared.ID]string, error) {
	out := map[shared.ID]string{}
	for _, actorID := range actorIDs {
		if pseudonym, found := d.pseudonym[actorID]; found {
			out[actorID] = pseudonym
		}
	}
	return out, nil
}

var (
	_ Requests   = (*double)(nil)
	_ Consents   = (*double)(nil)
	_ Subjects   = (*double)(nil)
	_ Pseudonyms = (*double)(nil)
)

// The zero filter is "what do we still owe", which is what somebody opening the list is asking.
func TestTheZeroFilterAnswersWhatIsOpen(t *testing.T) {
	requests := newDouble()
	open := domain.Request{
		ID:     shared.MustParseID("0192f000-0000-7000-8000-0000000000b1"),
		Status: domain.StatusReceived, DueAt: time.Now().Add(time.Hour),
	}
	closed := domain.Request{
		ID:     shared.MustParseID("0192f000-0000-7000-8000-0000000000b2"),
		Status: domain.StatusCompleted,
	}
	for _, request := range []domain.Request{open, closed} {
		if err := requests.Insert(t.Context(), request); err != nil {
			t.Fatalf("recording: %v", err)
		}
	}

	page, err := requests.List(t.Context(), Filter{})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(page.Requests) != 1 || page.Requests[0].ID != open.ID {
		t.Errorf("the zero filter answered %d cases", len(page.Requests))
	}

	whole, err := requests.List(t.Context(), Filter{IncludeClosed: true})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(whole.Requests) != 2 {
		t.Errorf("the whole list answered %d cases", len(whole.Requests))
	}
}

// Saving a case somebody deleted under the caller answers false rather than failing: it is a row
// that is not there rather than a write that did not work.
func TestSavingACaseThatIsNotThereAnswersFalse(t *testing.T) {
	requests := newDouble()

	saved, err := requests.Save(t.Context(), domain.Request{
		ID: shared.MustParseID("0192f000-0000-7000-8000-0000000000b3"),
	})
	if err != nil {
		t.Fatalf("saving: %v", err)
	}
	if saved {
		t.Error("a case that was never recorded reported as saved")
	}
}

// The pseudonym is idempotent on the actor, because an erasure is a job and a job is retried.
func TestAPseudonymIsRecordedOnce(t *testing.T) {
	pseudonyms := newDouble()
	actor := shared.MustParseID("0192f000-0000-7000-8000-0000000000b4")

	for _, name := range []string{"former-user-1", "former-user-2"} {
		if err := pseudonyms.Assign(t.Context(), actor, name, "DSR_ERASURE", time.Now()); err != nil {
			t.Fatalf("assigning: %v", err)
		}
	}

	found, err := pseudonyms.For(t.Context(), []shared.ID{actor})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if found[actor] != "former-user-1" {
		t.Errorf("the actor is answered as %q - a retried erasure renamed them", found[actor])
	}
}
