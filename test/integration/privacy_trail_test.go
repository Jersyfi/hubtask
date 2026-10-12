// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/audit"
	auditservice "github.com/Jersyfi/hubtask/core/application/service/audit"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/audit"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// UC-PRV-03 check 6 end to end: a person's trail entries outlive their erasure, the read and both
// export formats answer one pseudonym for them and never the name or the address, and the chain
// the erasure did not touch still verifies.

func TestAnErasedPersonsTrailReadsAndExportsUnderOnePseudonymAndStillVerifies(t *testing.T) {
	ctx := context.Background()
	email := "anna-" + freshID(t).String() + "@example.org"
	subject := seedSubject(ctx, t, email)
	colleague := freshID(t)

	// The person's own entries, one labelled with the name and one with the address - a failed
	// sign-in names the address that was tried - and a colleague's entry that must keep its label.
	entry := func(actor shared.ID, label string, action port.Action, offset time.Duration) port.Entry {
		return port.Entry{
			TenantID: subject.tenant, OccurredAt: created.Add(offset),
			Action: action, Outcome: port.OutcomeSuccess, Severity: port.SeverityInfo,
			ActorKind: shared.ActorUser, ActorID: actor, ActorLabel: label,
			TargetType: "container", TargetID: subject.hub,
		}
	}
	appendTo(ctx, t, subject.tenant, []port.Entry{
		entry(subject.account, "Anna Beispiel", "container.renamed", time.Second),
		entry(subject.account, email, "auth.login_failed", 2*time.Second),
		entry(colleague, "Carla Kollegin", "container.renamed", 3*time.Second),
	})

	if _, err := eraserFor(t).Erase(ctx, subjectActor(subject),
		erasureRequest(ctx, t, subject, domain.ModeAnonymize)); err != nil {
		t.Fatalf("erasing: %v", err)
	}

	// The entries remain, as rows: the erasure pseudonymises at the boundary and edits nothing.
	if rows := countIn(ctx, t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND actor_id = $2`,
		subject.tenant.String(), subject.account.String()); rows != 2 {
		t.Fatalf("%d of the person's two trail entries remain", rows)
	}

	period := repository.Period{From: created.Add(-time.Hour), To: created.Add(time.Hour)}
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))

	// The read.
	page, err := auditservice.ListAuditEntries{
		Trail: auditTrailRepo(), Authorizer: permissive{}, Pseudonyms: privacyRepo(),
		UnitOfWork: unitOfWork,
	}.Execute(ctx, auditActor(subject.tenant), auditservice.EntryQuery{
		Filter: repository.Filter{ActorID: subject.account},
	})
	if err != nil {
		t.Fatalf("reading the trail: %v", err)
	}
	if len(page.Records) != 2 {
		t.Fatalf("the read answers %d of the person's entries, want 2", len(page.Records))
	}
	pseudonym := page.Records[0].Entry.ActorLabel
	for _, record := range page.Records {
		label := record.Entry.ActorLabel
		if label != pseudonym {
			t.Errorf("one read answers the person as %q and as %q", pseudonym, label)
		}
		if label == "" || strings.Contains(label, "Anna") || strings.Contains(label, email) {
			t.Errorf("the read answers the erased person as %q", label)
		}
	}

	// Both export formats, through the archivist the export job runs.
	for _, format := range []auditservice.Format{auditservice.FormatJSONL, auditservice.FormatCSV} {
		target := &memoryTarget{written: map[string][]byte{}}
		manifest, err := auditservice.Archivist{
			Trail: auditTrailRepo(), Targets: target, Pseudonyms: privacyRepo(),
			Encryptor: keylessInstallation{}, UnitOfWork: unitOfWork,
			Clock: portclock.Fixed(created), ProductVersion: "integration",
		}.Write(ctx, auditservice.ArchiveRequest{
			ExportID: freshID(t), TenantID: subject.tenant, TargetID: freshID(t),
			Period: period, Format: format,
		})
		if err != nil {
			t.Fatalf("exporting as %s: %v", format, err)
		}

		member := manifest.Member
		content := string(target.member(t, member))
		if strings.Contains(content, "Anna Beispiel") || strings.Contains(content, email) {
			t.Errorf("the %s export carries the erased person's name or address", format)
		}
		if !strings.Contains(content, "Carla Kollegin") {
			t.Errorf("the %s export lost a colleague's label", format)
		}
		labels := exportedLabelsOf(t, format, content, subject.account)
		if len(labels) != 2 {
			t.Fatalf("the %s export holds %d of the person's entries, want 2", format, len(labels))
		}
		for _, label := range labels {
			if label != pseudonym {
				t.Errorf("the %s export answers the person as %q, the read as %q", format, label, pseudonym)
			}
		}
	}

	// And the chain: the erasure's own entry was appended to it, nothing in it was rewritten.
	found, err := verifierFor(t).Execute(ctx, auditActor(subject.tenant), period)
	if err != nil {
		t.Fatalf("verifying: %v", err)
	}
	if !found.Valid || found.GapCount != 0 || found.FirstBrokenSeq != 0 {
		t.Fatalf("the chain does not verify after the erasure: %+v", found)
	}
	if found.Checked < 4 {
		t.Errorf("%d entries were checked, want the three written and the erasure's own", found.Checked)
	}
}

// exportedLabelsOf answers the actor label of every exported entry of one actor.
func exportedLabelsOf(t *testing.T, format auditservice.Format, content string, actor shared.ID) []string {
	t.Helper()

	var labels []string
	if format == auditservice.FormatCSV {
		rows, err := csv.NewReader(strings.NewReader(content)).ReadAll()
		if err != nil || len(rows) == 0 {
			t.Fatalf("the CSV export does not parse: %v", err)
		}
		column := map[string]int{}
		for index, name := range rows[0] {
			column[name] = index
		}
		for _, row := range rows[1:] {
			if row[column["actor_id"]] == actor.String() {
				labels = append(labels, row[column["actor_label"]])
			}
		}
		return labels
	}

	for _, line := range strings.Split(strings.TrimSpace(content), "\n") {
		var exported struct {
			Actor struct {
				ID    string `json:"id"`
				Label string `json:"label"`
			} `json:"actor"`
		}
		if err := json.Unmarshal([]byte(line), &exported); err != nil {
			t.Fatalf("an exported line is not JSON: %v", err)
		}
		if exported.Actor.ID == actor.String() {
			labels = append(labels, exported.Actor.Label)
		}
	}
	return labels
}
