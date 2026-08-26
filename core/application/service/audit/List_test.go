// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package audit

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/audit"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domainservice "github.com/Jersyfi/hubtask/core/domain/service"
	port "github.com/Jersyfi/hubtask/core/port/audit"
)

// Reading the trail (E-09, audit.md §5). What is under test is the access model, which is §5's
// rather than the ordinary role matrix, and the narrowing that goes with it.

func newListHarness(whole bool) (ListAuditEntries, *trailStore, *authorizerDouble, *unitOfWork) {
	trail := &trailStore{}
	authorizer := &authorizerDouble{permits: whole}
	uow := &unitOfWork{}
	return ListAuditEntries{Trail: trail, Authorizer: authorizer, UnitOfWork: uow}, trail, authorizer, uow
}

func TestAnAdministratorReadsTheWholeTrailUnnarrowed(t *testing.T) {
	list, trail, authorizer, uow := newListHarness(true)

	_, err := list.Execute(context.Background(), actor(), EntryQuery{
		Filter: repository.Filter{ActionPrefix: "auth."},
	})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}

	if len(trail.asked) != 1 {
		t.Fatalf("the trail was asked %d times", len(trail.asked))
	}
	if !trail.asked[0].ActorID.IsZero() {
		t.Errorf("the whole trail was narrowed to %s", trail.asked[0].ActorID)
	}
	if permissionOf(authorizer.requests[0]) != domainservice.PermissionAuditRead {
		t.Errorf("the read asked for %s", authorizer.requests[0].Permission)
	}
	if authorizer.requests[0].TokenScope != auditRead {
		t.Errorf("the read asked for the scope %q", authorizer.requests[0].TokenScope)
	}
	if uow.reads != 1 || uow.writes != 0 {
		t.Errorf("a read opened %d read and %d write transactions", uow.reads, uow.writes)
	}
}

// §5's second row: a member reads their own events. Transparency towards the employee rather than
// a lesser administrator's view - and the narrowing is applied here rather than trusted from the
// request, which is the half a filter could otherwise leave out.
func TestAMemberIsNarrowedToTheirOwnEvents(t *testing.T) {
	list, trail, authorizer, _ := newListHarness(false)

	if _, err := list.Execute(context.Background(), actor(), EntryQuery{}); err != nil {
		t.Fatalf("reading: %v", err)
	}

	if trail.asked[0].ActorID != accountID {
		t.Errorf("the page was narrowed to %s", trail.asked[0].ActorID)
	}
	// The ordinary permission: holding a role in the workspace at all is the whole condition for
	// reading what you did yourself.
	if permissionOf(authorizer.requests[0]) != domainservice.PermissionRead {
		t.Errorf("reading one's own events asked for %s", authorizer.requests[0].Permission)
	}
}

func TestAMemberNamingThemselvesIsNotRefused(t *testing.T) {
	list, trail, _, _ := newListHarness(false)

	_, err := list.Execute(context.Background(), actor(), EntryQuery{
		Filter: repository.Filter{ActorID: accountID},
	})
	if err != nil {
		t.Fatalf("reading one's own events: %v", err)
	}
	if trail.asked[0].ActorID != accountID {
		t.Errorf("the page was narrowed to %s", trail.asked[0].ActorID)
	}
}

// Asking about a colleague without the right to the whole trail is refused rather than quietly
// narrowed: answering "your own events" to a question about somebody else would say that the
// colleague did nothing, which is a different and false answer.
func TestAskingAboutSomebodyElseIsRefusedRatherThanNarrowed(t *testing.T) {
	list, trail, authorizer, _ := newListHarness(false)
	authorizer.refuse = shared.ErrForbidden.WithDetail("access.not_permitted")

	_, err := list.Execute(context.Background(), actor(), EntryQuery{
		Filter: repository.Filter{ActorID: colleagueID},
	})
	if err == nil {
		t.Fatal("a member read a colleague's events")
	}
	if len(trail.asked) != 0 {
		t.Errorf("the trail was read %d times after a refusal", len(trail.asked))
	}
	// The refusal is recorded against reading the trail, which is what was attempted.
	if permissionOf(authorizer.requests[0]) != domainservice.PermissionAuditRead {
		t.Errorf("the refusal was recorded against %s", authorizer.requests[0].Permission)
	}
	if authorizer.requests[0].Action != ReadAction {
		t.Errorf("the refusal was recorded against the action %q", authorizer.requests[0].Action)
	}
}

// A successful read writes nothing. The trail would otherwise grow by being read, and the second
// page would contain the reading of the first.
func TestAPermittedReadRecordsNothing(t *testing.T) {
	list, _, authorizer, _ := newListHarness(true)

	if _, err := list.Execute(context.Background(), actor(), EntryQuery{}); err != nil {
		t.Fatalf("reading: %v", err)
	}
	if len(authorizer.permitOn) != 1 {
		t.Fatalf("the whole-trail question was asked %d times", len(authorizer.permitOn))
	}
	if permissionOf(wholeTrailRequest()) != domainservice.PermissionAuditRead {
		t.Error("the whole-trail request no longer asks for the audit permission")
	}
}

func TestAPeriodThatEndsBeforeItStartsIsRefused(t *testing.T) {
	list, _, _, _ := newListHarness(true)

	_, err := list.Execute(context.Background(), actor(), EntryQuery{
		Filter: repository.Filter{From: now, To: now.Add(-time.Hour)},
	})
	if err == nil {
		t.Fatal("a period ending before it starts was accepted")
	}
}

func TestAnUnknownOutcomeIsRefused(t *testing.T) {
	list, _, _, _ := newListHarness(true)

	_, err := list.Execute(context.Background(), actor(), EntryQuery{
		Filter: repository.Filter{Outcome: port.Outcome("MAYBE")},
	})
	if err == nil {
		t.Fatal("an outcome that does not exist was accepted")
	}
}

func TestThePageSizeIsClamped(t *testing.T) {
	list, trail, _, _ := newListHarness(true)

	for _, c := range []struct{ asked, want int }{
		{0, DefaultPageSize}, {10, 10}, {5000, MaxPageSize},
	} {
		if _, err := list.Execute(context.Background(), actor(), EntryQuery{
			Filter: repository.Filter{Page: repository.Page{Size: c.asked}},
		}); err != nil {
			t.Fatalf("reading: %v", err)
		}
		if got := trail.asked[len(trail.asked)-1].Page.Size; got != c.want {
			t.Errorf("a size of %d was read as %d, want %d", c.asked, got, c.want)
		}
	}
}

// The projection: what an entry looks like on the way out, and what it must not contain.
func TestTheProjectionMasksWhatTheEntryMasked(t *testing.T) {
	list, trail, _, _ := newListHarness(true)
	trail.records = []repository.Record{
		record("0192f000-0000-7000-8000-0000000000b1", 7, func(*repository.Record) {}),
	}

	out, err := list.Descriptor().Handler.Invoke(context.Background(), actor(), usecase.Input{})
	if err != nil {
		t.Fatalf("invoking: %v", err)
	}
	rows, _ := out["data"].([]usecase.Output)
	if len(rows) != 1 {
		t.Fatalf("%d entries came back", len(rows))
	}

	changes, _ := rows[0]["changes"].([]map[string]any)
	found := map[string]map[string]any{}
	for _, change := range changes {
		field, _ := change["field"].(string)
		found[field] = change
	}

	if _, secret := found["token"]; secret {
		t.Error("a SECRET field reached the projection")
	}
	if found["title"]["to"] != nil {
		t.Error("a SENSITIVE value reached the projection in clear text")
	}
	if found["title"]["changed"] != true || found["title"]["to_hash"] == nil {
		t.Error("a SENSITIVE field lost the fingerprint that makes two entries comparable")
	}
	if found["status"]["to"] != "DONE" || found["status"]["from"] != "OPEN" {
		t.Errorf("an OPEN field came back as %v", found["status"])
	}
	if rows[0]["seq"] != int64(7) {
		t.Errorf("the sequence number came back as %v", rows[0]["seq"])
	}
	if rows[0]["hash"] != "abcd" {
		t.Errorf("the hash came back as %v", rows[0]["hash"])
	}
}

// The registry validates an input against the declared fields before a handler sees it, and every
// service test below this line calls Execute directly - so the shape the REST controller builds is
// asserted here, against the descriptor itself.
func TestTheDescriptorDeclaresEveryFieldTheControllerSends(t *testing.T) {
	descriptor := ListAuditEntries{}.Descriptor()

	in := usecase.Input{
		"from":        now.Format(time.RFC3339Nano),
		"to":          now.Add(time.Hour).Format(time.RFC3339Nano),
		"action":      "auth.",
		"actor_id":    accountID.String(),
		"target_type": "container",
		"target_id":   targetID.String(),
		"outcome":     string(port.OutcomeDenied),
		"cursor":      "opaque",
		"size":        50,
	}
	if err := descriptor.ValidateInput(in); err != nil {
		t.Fatalf("the input the REST controller builds is refused: %v", err)
	}
	if !descriptor.ReadOnly {
		t.Error("reading the trail is declared as writing")
	}
	if descriptor.TokenScope != auditRead {
		t.Errorf("the read declares the scope %q", descriptor.TokenScope)
	}
}

// The handler is what the three channels actually call, and it is where the request becomes a
// filter. Every field is asserted on the way through, because a field parsed into the wrong place
// is a filter that silently answers about something else.
func TestEveryDeclaredFieldBecomesAFilter(t *testing.T) {
	list, trail, _, _ := newListHarness(true)

	_, err := list.Descriptor().Handler.Invoke(context.Background(), actor(), usecase.Input{
		"from":        now.Format(time.RFC3339),
		"to":          now.Add(time.Hour).Format(time.RFC3339),
		"action":      "auth.",
		"actor_id":    colleagueID.String(),
		"target_type": "container",
		"target_id":   targetID.String(),
		"outcome":     string(port.OutcomeDenied),
		"cursor":      "opaque",
		"size":        25,
	})
	if err != nil {
		t.Fatalf("invoking: %v", err)
	}

	asked := trail.asked[0]
	switch {
	case !asked.From.Equal(now), !asked.To.Equal(now.Add(time.Hour)):
		t.Errorf("the period arrived as %s..%s", asked.From, asked.To)
	case asked.ActionPrefix != "auth.":
		t.Errorf("the action prefix arrived as %q", asked.ActionPrefix)
	case asked.ActorID != colleagueID, asked.TargetID != targetID:
		t.Errorf("the actor or target arrived as %s / %s", asked.ActorID, asked.TargetID)
	case asked.TargetType != "container":
		t.Errorf("the target type arrived as %q", asked.TargetType)
	case asked.Outcome != port.OutcomeDenied:
		t.Errorf("the outcome arrived as %q", asked.Outcome)
	case asked.Page.Cursor != "opaque", asked.Page.Size != 25:
		t.Errorf("the page arrived as %+v", asked.Page)
	}
}

func TestAMalformedInstantIsRefusedWithItsField(t *testing.T) {
	list, _, _, _ := newListHarness(true)

	for _, field := range []string{"from", "to"} {
		_, err := list.Descriptor().Handler.Invoke(context.Background(), actor(), usecase.Input{
			field: "yesterday",
		})
		if err == nil {
			t.Fatalf("%q was accepted as an instant", field)
		}
		var problem *shared.Error
		if !errors.As(err, &problem) || problem.DetailCode != "audit."+field+"_malformed" {
			t.Errorf("%q was refused as %v", field, err)
		}
	}
}

// A malformed identifier is refused before anything is read, and named. The registry checks the
// declared shape, but a channel that hands the catalogue an input directly - an automation rule -
// reaches the handler's own parsing.
func TestAMalformedIdentifierIsRefused(t *testing.T) {
	list, trail, _, _ := newListHarness(true)

	for _, field := range []string{"actor_id", "target_id"} {
		if _, err := list.Descriptor().Handler.Invoke(context.Background(), actor(), usecase.Input{
			field: "not-an-identifier",
		}); err == nil {
			t.Errorf("%q was accepted as an identifier", field)
		}
	}
	if len(trail.asked) != 0 {
		t.Errorf("the trail was read %d times with a malformed filter", len(trail.asked))
	}
}

// The boundary substitution E-09's §6 promised and E-10 built: once an erasure has taken an actor,
// the trail answers a pseudonym instead of the label it stored.

type pseudonymStore struct{ names map[shared.ID]string }

func (p pseudonymStore) For(
	_ context.Context, actorIDs []shared.ID,
) (map[shared.ID]string, error) {
	out := map[shared.ID]string{}
	for _, actorID := range actorIDs {
		if name, found := p.names[actorID]; found {
			out[actorID] = name
		}
	}
	return out, nil
}

func TestAnErasedActorIsAnsweredAsAPseudonym(t *testing.T) {
	list, trail, _, _ := newListHarness(true)
	list.Pseudonyms = pseudonymStore{names: map[shared.ID]string{accountID: "former-user-0a2"}}
	trail.records = []repository.Record{
		record("0192f000-0000-7000-8000-0000000000b1", 1, func(*repository.Record) {}),
		record("0192f000-0000-7000-8000-0000000000b2", 2, func(r *repository.Record) {
			r.Entry.ActorID = colleagueID
			r.Entry.ActorLabel = "Bert Beispiel"
		}),
	}

	page, err := list.Execute(context.Background(), actor(), EntryQuery{})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}

	if page.Records[0].Entry.ActorLabel != "former-user-0a2" {
		t.Errorf("the erased actor is answered as %q", page.Records[0].Entry.ActorLabel)
	}
	// The identifier stays: it is what lets an auditor tell one actor's entries from another's,
	// and it was never a name.
	if page.Records[0].Entry.ActorID != accountID {
		t.Errorf("the entry lost its actor: %s", page.Records[0].Entry.ActorID)
	}
	// Everybody else is untouched.
	if page.Records[1].Entry.ActorLabel != "Bert Beispiel" {
		t.Errorf("an actor nobody erased is answered as %q", page.Records[1].Entry.ActorLabel)
	}
}

// An installation with no substitutions pays for nothing: the page comes back as it was read.
func TestATrailWithNoErasuresIsAnsweredAsItIs(t *testing.T) {
	list, trail, _, _ := newListHarness(true)
	list.Pseudonyms = pseudonymStore{}
	trail.records = []repository.Record{
		record("0192f000-0000-7000-8000-0000000000b1", 1, func(*repository.Record) {}),
	}

	page, err := list.Execute(context.Background(), actor(), EntryQuery{})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if page.Records[0].Entry.ActorLabel != "Anna Beispiel" {
		t.Errorf("the label came back as %q", page.Records[0].Entry.ActorLabel)
	}
}
