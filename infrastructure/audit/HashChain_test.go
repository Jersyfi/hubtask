// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package audit

import (
	"bytes"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/audit"
)

var (
	tenantID = shared.MustParseID("0192f000-0000-7000-8000-00000000000a")
	entryID  = shared.MustParseID("0192f000-0000-7000-8000-0000000000e1")
	occurred = time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC)
)

func entry() port.Entry {
	return port.Entry{
		TenantID: tenantID, OccurredAt: occurred, Action: "container.created",
		Outcome: port.OutcomeSuccess, Severity: port.SeverityInfo,
		ActorKind: shared.ActorUser, ActorLabel: "Anna Beispiel",
		TargetType: "container",
		Changes: port.Changes(
			port.Change{Field: "type", Classification: port.Open, To: "HUB"},
			port.Change{Field: "name", Classification: port.Sensitive, To: "Private"},
		),
	}
}

func hash(t *testing.T, previous []byte, seq int64, e port.Entry) []byte {
	t.Helper()
	digest, err := Chain(previous, entryID, seq, e)
	if err != nil {
		t.Fatalf("chaining failed: %v", err)
	}
	return digest
}

// The same entry hashes the same way every time, or verification would report a break on a
// perfectly intact chain.
func TestTheHashIsDeterministic(t *testing.T) {
	first := hash(t, nil, 1, entry())
	second := hash(t, nil, 1, entry())

	if !bytes.Equal(first, second) {
		t.Error("two runs over the same entry produced different hashes")
	}
	if len(first) != 32 {
		t.Errorf("the hash is %d bytes, want 32 (SHA-256)", len(first))
	}
}

// What the chain is for: changing anything about a recorded entry changes its hash, so a row
// rewritten in place no longer matches the one after it.
func TestEveryFieldIsCovered(t *testing.T) {
	original := hash(t, nil, 1, entry())

	cases := map[string]func(*port.Entry){
		"the action":      func(e *port.Entry) { e.Action = "container.deleted" },
		"the outcome":     func(e *port.Entry) { e.Outcome = port.OutcomeDenied },
		"the severity":    func(e *port.Entry) { e.Severity = port.SeverityCritical },
		"the actor kind":  func(e *port.Entry) { e.ActorKind = shared.ActorSystem },
		"the actor label": func(e *port.Entry) { e.ActorLabel = "Somebody Else" },
		"the actor":       func(e *port.Entry) { e.ActorID = entryID },
		"the target":      func(e *port.Entry) { e.TargetID = entryID },
		"the time":        func(e *port.Entry) { e.OccurredAt = occurred.Add(time.Second) },
		"the tenant":      func(e *port.Entry) { e.TenantID = entryID },
		"the request":     func(e *port.Entry) { e.Context.RequestID = "01J9" },
		"the changes": func(e *port.Entry) {
			e.Changes = port.Changes(port.Change{Field: "type", Classification: port.Open, To: "COLLECTION"})
		},
		"the legal basis": func(e *port.Entry) { e.LegalBasis = "dsr.erasure" },
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			tampered := entry()
			mutate(&tampered)

			if bytes.Equal(original, hash(t, nil, 1, tampered)) {
				t.Errorf("changing %s does not change the hash", name)
			}
		})
	}
}

// The position and the predecessor are part of the hash, which is what makes a chain a chain:
// removing an entry, or reordering two, breaks every hash after it.
func TestThePositionAndThePredecessorAreCovered(t *testing.T) {
	first := hash(t, nil, 1, entry())

	if bytes.Equal(first, hash(t, nil, 2, entry())) {
		t.Error("the sequence number is not covered - an entry could be moved")
	}
	if bytes.Equal(hash(t, first, 2, entry()), hash(t, nil, 2, entry())) {
		t.Error("the previous hash is not covered - an entry could be removed")
	}
}

// The canonical form is what a verifier recomputes, so it has to be readable and stable rather
// than whatever a struct happens to serialise to.
func TestTheCanonicalFormNamesItsFields(t *testing.T) {
	canonical, err := Canonical(entryID, 7, entry())
	if err != nil {
		t.Fatalf("serialising failed: %v", err)
	}

	for _, field := range []string{
		`"id":`, `"tenant_id":`, `"seq":7`, `"occurred_at":`, `"action":"container.created"`,
		`"outcome":"SUCCESS"`, `"actor_type":"USER"`, `"changes":`,
	} {
		if !bytes.Contains(canonical, []byte(field)) {
			t.Errorf("%s is missing from the canonical form: %s", field, canonical)
		}
	}
	// The masking happened before the entry got here, and the chain must not undo it.
	if bytes.Contains(canonical, []byte(`"Private"`)) {
		t.Errorf("a sensitive value reached the canonical form: %s", canonical)
	}
}
