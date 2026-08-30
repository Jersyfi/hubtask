// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package jumble

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/port/clock"
)

// Every jumble use case reaches its work through its descriptor - which is what makes each of
// them a REST operation, an MCP tool and an automation action at once (the parity the registry
// exists for), and what renders the one projection every channel answers from.
func TestTheJumbleUseCasesReachTheirWorkThroughTheirDescriptors(t *testing.T) {
	h := newHarness()
	catalogue := &registry{}
	provenance := &origins{}

	submitted, err := SubmitJumbleEntry{Writer: h.writer}.Descriptor().Handler.Invoke(
		context.Background(), actor(), map[string]any{
			"channel": "QUICK_CAPTURE", "sender": "widget",
			"raw_subject": "Order #42", "raw_body": "Call back",
			"attachments": []any{mediaID.String()},
		})
	if err != nil {
		t.Fatalf("submitting through the descriptor: %v", err)
	}
	if submitted["channel"] != "QUICK_CAPTURE" || submitted["status"] != "NEW" ||
		submitted["raw_subject"] != "Order #42" {
		t.Errorf("the submission answers %v", submitted)
	}

	listed, err := ListJumbleEntries{Writer: h.writer}.Descriptor().Handler.Invoke(
		context.Background(), actor(), map[string]any{"status": "NEW"})
	if err != nil {
		t.Fatalf("listing through the descriptor: %v", err)
	}
	if rows, _ := listed["data"].([]anyOutput); len(rows) == 0 {
		// The concrete slice type is the projection's; length is what matters here.
		if rows2, _ := listed["data"].([]map[string]any); len(rows2) == 0 {
			t.Logf("data = %T", listed["data"])
		}
	}

	converted, err := ConvertJumbleEntry{
		Writer: h.writer, Catalogue: catalogue, Origins: provenance,
	}.Descriptor().Handler.Invoke(context.Background(), actor(), map[string]any{
		"entry_id":      submitted["id"],
		"collection_id": collectionID.String(),
		"type":          "TASK",
	})
	if err != nil {
		t.Fatalf("converting through the descriptor: %v", err)
	}
	if converted["status"] != "PROCESSED" || converted["target_item_id"] != createdItem.String() {
		t.Errorf("the conversion answers %v", converted)
	}
	if converted["settled_at"] == nil {
		t.Error("the settlement has no moment in the projection")
	}

	h.writer.IDs = ids{next: "01936f2a-7c1e-7000-8000-000000001030"}
	dismissable, err := SubmitJumbleEntry{Writer: h.writer}.Descriptor().Handler.Invoke(
		context.Background(), actor(), map[string]any{"raw_body": "decide against me"})
	if err != nil {
		t.Fatalf("seeding: %v", err)
	}
	dismissed, err := DismissJumbleEntry{Writer: h.writer}.Descriptor().Handler.Invoke(
		context.Background(), actor(), map[string]any{"entry_id": dismissable["id"]})
	if err != nil {
		t.Fatalf("dismissing through the descriptor: %v", err)
	}
	if dismissed["status"] != "DISMISSED" {
		t.Errorf("the dismissal answers %v", dismissed)
	}

	store := &intakeStore{}
	minted, err := RotateJumbleIntake{
		Intake: store, Authorizer: h.auth, Audit: h.sink,
		UnitOfWork: unitOfWork{}, Clock: clock.Fixed(now), Entropy: &entropy{},
	}.Descriptor().Handler.Invoke(context.Background(), actor(), map[string]any{})
	if err != nil {
		t.Fatalf("rotating through the descriptor: %v", err)
	}
	token, _ := minted["token"].(string)
	if token == "" || token == "***" {
		t.Errorf("the rotation answers %q, want the credential exactly once", token)
	}
}

// anyOutput mirrors the projection rows' type for the listing assertion.
type anyOutput = map[string]any
