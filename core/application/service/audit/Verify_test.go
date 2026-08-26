// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package audit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/audit"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// Verifying the chain (E-09, audit.md §3, test AT-2). What is under test here is the arithmetic of
// the walk - the break, the link, the gap - against a chain built in memory. That the *stored*
// chain holds over a thousand mixed events, and that a row edited in the database is found, is a
// question for a real PostgreSQL and is asked in test/integration.

// chainDouble is a hash function with the one property the real one has: the same inputs give the
// same bytes, and any different input gives different bytes.
type chainDouble struct{}

func (chainDouble) Link(previousHash []byte, id shared.ID, seq int64, entry port.Entry) ([]byte, error) {
	digest := sha256.New()
	digest.Write(previousHash)
	digest.Write([]byte(id))
	_ = binary.Write(digest, binary.BigEndian, seq)
	digest.Write([]byte(entry.Action))
	return digest.Sum(nil), nil
}

// chainOf builds an intact chain of n entries, exactly as the sink would have written it.
func chainOf(n int) []repository.Record {
	var records []repository.Record
	var previous []byte

	for i := 1; i <= n; i++ {
		entry := port.Entry{
			TenantID: tenantID, OccurredAt: now.Add(time.Duration(i) * time.Second),
			Action: "container.created", Outcome: port.OutcomeSuccess, Severity: port.SeverityInfo,
			ActorKind: shared.ActorKind("USER"), ActorID: accountID,
		}
		id := shared.MustParseID("0192f000-0000-7000-8000-0000000001" + string("0123456789abcdef"[i%16]) + string("0123456789abcdef"[i/16%16]))
		hash, _ := chainDouble{}.Link(previous, id, int64(i), entry)
		records = append(records, repository.Record{
			ID: id, Seq: int64(i), Entry: entry, PrevHash: previous, Hash: hash,
		})
		previous = hash
	}
	return records
}

func newVerifyHarness(records []repository.Record) (VerifyAuditChain, *trailStore, *auditSink, *authorizerDouble) {
	trail := &trailStore{records: records}
	sink := &auditSink{}
	authorizer := &authorizerDouble{permits: true}
	return VerifyAuditChain{
		Trail: trail, Chain: chainDouble{}, Authorizer: authorizer, Audit: sink,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now),
	}, trail, sink, authorizer
}

func period() repository.Period {
	return repository.Period{From: now, To: now.Add(24 * time.Hour)}
}

func TestAnIntactChainVerifiesAndRecordsNothing(t *testing.T) {
	verify, _, sink, _ := newVerifyHarness(chainOf(50))

	found, err := verify.Execute(context.Background(), actor(), period())
	if err != nil {
		t.Fatalf("verifying: %v", err)
	}

	if !found.Valid || found.Checked != 50 {
		t.Errorf("an intact chain of 50 verified as %+v", found)
	}
	if found.FirstBrokenSeq != 0 || len(found.Gaps) != 0 {
		t.Errorf("an intact chain reported a break: %+v", found)
	}
	if len(sink.entries) != 0 {
		t.Errorf("a clean check wrote %d entries", len(sink.entries))
	}
	// Nothing anchors yet, so the honest answer is that nothing is sealed.
	if !found.SealedUntil.IsZero() {
		t.Errorf("an installation with no anchor claims to be sealed until %s", found.SealedUntil)
	}
}

// A row somebody edited in the database hashes to something else, whatever else they remembered to
// change - which is the whole of what the chain is for.
func TestATamperedEntryIsFoundAtItsOwnSequenceNumber(t *testing.T) {
	records := chainOf(10)
	records[4].Entry.Action = "container.deleted"

	verify, _, sink, _ := newVerifyHarness(records)

	found, err := verify.Execute(context.Background(), actor(), period())
	if err != nil {
		t.Fatalf("verifying: %v", err)
	}
	if found.Valid {
		t.Fatal("a rewritten entry verified as intact")
	}
	if found.FirstBrokenSeq != 5 {
		t.Errorf("the break was reported at %d, want 5", found.FirstBrokenSeq)
	}
	if found.Checked != 10 {
		t.Errorf("%d entries were checked; the walk stops at nothing", found.Checked)
	}

	// The one entry a verification writes, and it is critical.
	if len(sink.entries) != 1 {
		t.Fatalf("a break wrote %d entries", len(sink.entries))
	}
	entry := sink.entries[0]
	if entry.Action != ChainBrokenAction || entry.Severity != port.SeverityCritical {
		t.Errorf("the break was recorded as %s / %s", entry.Action, entry.Severity)
	}
	if entry.Outcome != port.OutcomeFailed {
		t.Errorf("the break was recorded with the outcome %s", entry.Outcome)
	}
}

// An entry whose own hash is fine but whose predecessor's digest is not the one before it is an
// entry that was moved, or a predecessor that was removed. A per-row check cannot see it.
func TestABrokenLinkIsFoundEvenWhenEveryRowHashesCorrectly(t *testing.T) {
	records := chainOf(6)
	// Rebuild entry 4 as a self-consistent row that points at the wrong predecessor.
	records[3].PrevHash = []byte{0x99}
	records[3].Hash, _ = chainDouble{}.Link(records[3].PrevHash, records[3].ID, records[3].Seq, records[3].Entry)

	verify, _, _, _ := newVerifyHarness(records)

	found, err := verify.Execute(context.Background(), actor(), period())
	if err != nil {
		t.Fatalf("verifying: %v", err)
	}
	if found.Valid || found.FirstBrokenSeq != 4 {
		t.Errorf("a broken link verified as %+v", found)
	}
}

// The gap is the other half, and the reason `0001_init` leaves it to the application: a global
// UNIQUE (tenant_id, seq) cannot be enforced across partitions.
func TestAMissingEntryIsReportedAsAGap(t *testing.T) {
	records := chainOf(8)
	// Entries 4 and 5 are gone, and the rest is untouched - which is what a row somebody deleted
	// past the trigger would look like.
	removed := append(append([]repository.Record{}, records[:3]...), records[5:]...)

	verify, _, sink, _ := newVerifyHarness(removed)

	found, err := verify.Execute(context.Background(), actor(), period())
	if err != nil {
		t.Fatalf("verifying: %v", err)
	}
	if found.Valid {
		t.Fatal("a chain with two entries missing verified as intact")
	}
	if found.GapCount != 2 || len(found.Gaps) != 2 || found.Gaps[0] != 4 || found.Gaps[1] != 5 {
		t.Errorf("the gaps were reported as %v (%d)", found.Gaps, found.GapCount)
	}
	if len(sink.entries) != 1 {
		t.Errorf("a gap wrote %d entries", len(sink.entries))
	}
}

// A hole of a million entries would answer with a million integers; the client asking whether the
// trail is intact has its answer after the first.
func TestTheReportedGapsAreCutAndTheCountIsNot(t *testing.T) {
	records := chainOf(2)
	records[1].Seq = 1000
	records[1].Hash, _ = chainDouble{}.Link(records[1].PrevHash, records[1].ID, records[1].Seq, records[1].Entry)

	verify, _, _, _ := newVerifyHarness(records)

	found, err := verify.Execute(context.Background(), actor(), period())
	if err != nil {
		t.Fatalf("verifying: %v", err)
	}
	if found.GapCount != 998 {
		t.Errorf("%d missing entries were counted, want 998", found.GapCount)
	}
	if len(found.Gaps) != maxReportedGaps {
		t.Errorf("%d gaps were listed, want the cut at %d", len(found.Gaps), maxReportedGaps)
	}
}

func TestAnAnchoredChainReportsWhenItWasSealed(t *testing.T) {
	verify, trail, _, _ := newVerifyHarness(chainOf(3))
	trail.anchor = repository.Anchor{
		AnchoredAt: now.Add(-time.Hour), LastSeq: 3, ChainHash: []byte{0x01},
	}

	found, err := verify.Execute(context.Background(), actor(), period())
	if err != nil {
		t.Fatalf("verifying: %v", err)
	}
	if !found.SealedUntil.Equal(now.Add(-time.Hour)) {
		t.Errorf("the seal was reported as %s", found.SealedUntil)
	}
}

// A verification is always over the whole trail: one narrowed to an actor would be a chain with
// every other entry missing, and would answer "broken" for a trail that is intact.
func TestAVerificationAsksForTheWholeTrail(t *testing.T) {
	verify, _, _, authorizer := newVerifyHarness(chainOf(2))

	if _, err := verify.Execute(context.Background(), actor(), period()); err != nil {
		t.Fatalf("verifying: %v", err)
	}
	if authorizer.requests[0].Permission != wholeTrailRequest().Permission {
		t.Errorf("the check asked for %s", authorizer.requests[0].Permission)
	}
	if authorizer.requests[0].TokenScope != auditRead {
		t.Errorf("the check asked for the scope %q", authorizer.requests[0].TokenScope)
	}
}

func TestAVerificationNeedsBothEndsOfItsPeriod(t *testing.T) {
	verify, trail, _, _ := newVerifyHarness(chainOf(2))

	for _, incomplete := range []repository.Period{
		{To: now.Add(time.Hour)}, {From: now}, {From: now, To: now},
	} {
		if _, err := verify.Execute(context.Background(), actor(), incomplete); err == nil {
			t.Errorf("the period %+v was accepted", incomplete)
		}
	}
	if len(trail.walked) != 0 {
		t.Errorf("the trail was walked %d times for an incomplete period", len(trail.walked))
	}
}

func TestTheVerificationDescriptorTakesWhatTheControllerSends(t *testing.T) {
	descriptor := VerifyAuditChain{}.Descriptor()

	if err := descriptor.ValidateInput(usecase.Input{
		"from": now.Format(time.RFC3339Nano), "to": now.Add(time.Hour).Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("the input the REST controller builds is refused: %v", err)
	}
	if err := descriptor.ValidateInput(usecase.Input{"from": now.Format(time.RFC3339Nano)}); err == nil {
		t.Error("a verification without an end to its period was accepted")
	}
}

// The answer carries explicit nulls, so that a client can read the two fields unconditionally.
func TestTheAnswerSpellsOutWhatIsAbsent(t *testing.T) {
	out := VerificationOutput(Verification{Valid: true, Checked: 3})

	if out["first_broken_seq"] != nil || out["sealed_until"] != nil {
		t.Errorf("an intact chain answered %v", out)
	}
	if gaps, ok := out["gaps"].([]int64); !ok || len(gaps) != 0 {
		t.Errorf("the gaps came back as %v", out["gaps"])
	}
}

// The chain double is only a stand-in if it behaves like a hash: same in, same out; anything else
// in, something else out. A test that passed because the double collided would prove nothing.
func TestTheChainDoubleIsUsableAsAStandIn(t *testing.T) {
	entry := port.Entry{Action: "a"}
	first, _ := chainDouble{}.Link(nil, "id", 1, entry)
	same, _ := chainDouble{}.Link(nil, "id", 1, entry)
	other, _ := chainDouble{}.Link(nil, "id", 2, entry)

	if !bytes.Equal(first, same) || bytes.Equal(first, other) {
		t.Error("the double does not behave like a hash")
	}
}
