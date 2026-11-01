// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package media

import (
	"context"
	"io"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/port/storage"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// scanner answers what it was set to, after reading the whole stream it was handed.
type scanner struct {
	verdict storage.Verdict
	err     error
	read    []int
}

func (s *scanner) Scan(_ context.Context, content io.Reader) (storage.Verdict, error) {
	body, _ := io.ReadAll(content)
	s.read = append(s.read, len(body))
	return s.verdict, s.err
}

type enqueued struct{ requests []queue.Request }

func (e *enqueued) Enqueue(_ context.Context, request queue.Request) (shared.ID, error) {
	e.requests = append(e.requests, request)
	return "", nil
}

var scannerDown = shared.ErrUnavailable.WithDetail("dependency.scanner_unavailable")

// A clean scan seals the upload READY, and the scanner read every byte, not the head.
func TestAConfirmationScansTheWholeFile(t *testing.T) {
	handler, records, bytesOf, _, _ := confirmHarness(1 << 20)
	scan := &scanner{}
	handler.Scanner = scan
	staged(records, bytesOf, "image/png", int64(len(pngBytes)), pngBytes)

	sealed, err := handler.Execute(t.Context(), actor(mediaWrite), ConfirmCommand{MediaID: mintedID})
	if err != nil || sealed.Status != domain.StatusReady {
		t.Fatalf("a clean file was answered %+v, %v", sealed, err)
	}
	if len(scan.read) != 1 || scan.read[0] != len(pngBytes) {
		t.Errorf("the scanner read %v bytes, want the whole %d", scan.read, len(pngBytes))
	}
}

// A malicious file is refused by name and its bytes removed: nothing is sealed.
func TestAConfirmationRefusesAMaliciousFile(t *testing.T) {
	handler, records, bytesOf, _, _ := confirmHarness(1 << 20)
	handler.Scanner = &scanner{verdict: storage.Verdict{Malicious: true, Signature: "Eicar-Test-Signature"}}
	object := staged(records, bytesOf, "image/png", int64(len(pngBytes)), pngBytes)

	_, err := handler.Execute(t.Context(), actor(mediaWrite), ConfirmCommand{MediaID: mintedID})
	if shared.AsError(err).DetailCode != "media.infected" {
		t.Fatalf("a malicious file was answered %v", err)
	}
	if len(records.sealed) != 0 || len(bytesOf.deleted) != 1 || bytesOf.deleted[0] != object.StorageKey {
		t.Errorf("sealed %v, removed %v", records.sealed, bytesOf.deleted)
	}
}

// A scanner that cannot answer holds the file instead of refusing it or passing it, and asks for
// the scan to run again.
func TestAConfirmationHoldsAFileTheScannerCouldNotAnswerFor(t *testing.T) {
	handler, records, bytesOf, _, _ := confirmHarness(1 << 20)
	jobs := &enqueued{}
	handler.Scanner, handler.Jobs = &scanner{err: scannerDown}, jobs
	staged(records, bytesOf, "image/png", int64(len(pngBytes)), pngBytes)

	held, err := handler.Execute(t.Context(), actor(mediaWrite), ConfirmCommand{MediaID: mintedID})
	if err != nil || held.Status != domain.StatusPendingScan {
		t.Fatalf("a file nobody vouched for was answered %+v, %v", held, err)
	}
	if len(bytesOf.deleted) != 0 {
		t.Errorf("the held file's bytes were removed: %v", bytesOf.deleted)
	}
	if len(jobs.requests) != 1 || jobs.requests[0].Kind != queue.KindMediaScan || jobs.requests[0].TenantID != tenantID {
		t.Errorf("the scan was asked for as %+v", jobs.requests)
	}
	again, err := handler.Execute(t.Context(), actor(mediaWrite), ConfirmCommand{MediaID: mintedID})
	if err != nil || again.Status != domain.StatusPendingScan || len(records.sealed) != 1 {
		t.Errorf("confirming a held file again: %+v, %v, %d seals", again, err, len(records.sealed))
	}
}

// The scan job settles what it can: clean READY, malicious INFECTED with its bytes gone, and while
// the scanner stays down it leaves the rest held and says so.
func TestTheScanJobSettlesTheHeldFiles(t *testing.T) {
	records, bytesOf := newObjects(), newStore()
	for i, id := range []shared.ID{"0192f000-0000-7000-8000-0000000000b1", "0192f000-0000-7000-8000-0000000000b2"} {
		object := domain.Object{
			ID: id, TenantID: tenantID, StorageKey: "media/" + tenantID.String() + "/" + id.String(),
			ContentType: "application/pdf", ByteSize: 3, Usage: domain.UsageAttachment,
			Status: domain.StatusPendingScan, CreatedBy: accountID, CreatedAt: now.Add(-time.Duration(i) * time.Minute),
		}
		records.stored[id] = object
		bytesOf.content[object.StorageKey] = []byte("pdf")
	}
	scan := &scanner{err: scannerDown}
	job := ScanHeldMedia{Objects: records, Store: bytesOf, Scanner: scan, UnitOfWork: &unitOfWork{}}

	outcome, err := job.Execute(t.Context(), tenantID)
	if err != nil || outcome.Held != 2 || outcome.Settled != 0 {
		t.Fatalf("with the scanner down: %+v, %v", outcome, err)
	}

	scan.err, scan.verdict = nil, storage.Verdict{}
	outcome, err = job.Execute(t.Context(), tenantID)
	if err != nil || outcome.Held != 0 || outcome.Settled != 2 {
		t.Fatalf("with the scanner back: %+v, %v", outcome, err)
	}
	for id, object := range records.stored {
		if object.Status != domain.StatusReady {
			t.Errorf("%s is %s", id, object.Status)
		}
	}

	infected := records.stored["0192f000-0000-7000-8000-0000000000b1"]
	infected.Status = domain.StatusPendingScan
	records.stored[infected.ID] = infected
	scan.verdict = storage.Verdict{Malicious: true, Signature: "Eicar-Test-Signature"}
	if _, err := job.Execute(t.Context(), tenantID); err != nil {
		t.Fatal(err)
	}
	if records.stored[infected.ID].Status != domain.StatusInfected || bytesOf.content[infected.StorageKey] != nil {
		t.Errorf("a malicious held file is %s with bytes %q", records.stored[infected.ID].Status, bytesOf.content[infected.StorageKey])
	}
}

// A file that arrived whole - a mail's attachment - is scanned before it is stored: a malicious one
// is never stored, and one the scanner could not answer for is stored held.
func TestAnIngestScansBeforeItStores(t *testing.T) {
	for _, tc := range []struct {
		name   string
		scan   *scanner
		stored int
		status domain.Status
	}{
		{"clean", &scanner{}, 1, domain.StatusReady},
		{"malicious", &scanner{verdict: storage.Verdict{Malicious: true}}, 0, ""},
		{"no answer", &scanner{err: scannerDown}, 1, domain.StatusPendingScan},
	} {
		t.Run(tc.name, func(t *testing.T) {
			records, bytesOf, jobs := newObjects(), newStore(), &enqueued{}
			ingest := IngestMedia{
				Objects: records, Store: bytesOf, Guard: &guard{judged: "application/pdf"}, Jobs: jobs,
				UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), IDs: &ids{next: mintedID}, Config: config(1 << 20),
				Text: text.Composing{}, Scanner: tc.scan,
			}
			stored, err := ingest.Execute(t.Context(), tenantID, []IngestedFile{{FileName: "a.pdf", ClaimedType: "application/pdf", Content: []byte("pdf")}})
			if err != nil || len(stored) != tc.stored || len(bytesOf.puts) != tc.stored {
				t.Fatalf("answered %v, stored %d, %v", stored, len(bytesOf.puts), err)
			}
			if tc.stored == 1 && records.stored[mintedID].Status != tc.status {
				t.Errorf("the file is %s, want %s", records.stored[mintedID].Status, tc.status)
			}
			scans := 0
			for _, r := range jobs.requests {
				if r.Kind == queue.KindMediaScan {
					scans++
				}
			}
			if (tc.status == domain.StatusPendingScan) != (scans == 1) {
				t.Errorf("%d scan jobs asked for", scans)
			}
		})
	}
}

// sizeLimited refuses every stream longer than limit, as clamd's StreamMaxLength does, and passes
// the rest clean.
type sizeLimited struct{ limit int }

func (s sizeLimited) Scan(_ context.Context, content io.Reader) (storage.Verdict, error) {
	body, _ := io.ReadAll(content)
	if len(body) > s.limit {
		return storage.Verdict{}, domain.TooLargeToScan()
	}
	return storage.Verdict{}, nil
}

// A file the scanner refuses for its size is settled UNSCANNABLE, and the files behind it in the
// same pass are still scanned: the oldest oversized file never stops the workspace's scan.
func TestTheScanJobSettlesAFileTooLargeToScanAndGoesOn(t *testing.T) {
	records, bytesOf := newObjects(), newStore()
	large, small := shared.ID("0192f000-0000-7000-8000-0000000000c1"), shared.ID("0192f000-0000-7000-8000-0000000000c2")
	for i, held := range []struct {
		id    shared.ID
		bytes []byte
	}{{large, make([]byte, 30)}, {small, []byte("pdf")}} {
		object := domain.Object{
			ID: held.id, TenantID: tenantID, StorageKey: "media/" + tenantID.String() + "/" + held.id.String(),
			ContentType: "application/pdf", ByteSize: int64(len(held.bytes)), Usage: domain.UsageAttachment,
			Status: domain.StatusPendingScan, CreatedBy: accountID, CreatedAt: now.Add(time.Duration(i) * time.Minute),
		}
		records.stored[held.id] = object
		bytesOf.content[object.StorageKey] = held.bytes
	}
	job := ScanHeldMedia{Objects: records, Store: bytesOf, Scanner: sizeLimited{limit: 10}, UnitOfWork: &unitOfWork{}}

	outcome, err := job.Execute(t.Context(), tenantID)
	if err != nil || outcome.Settled != 2 || outcome.Held != 0 {
		t.Fatalf("one pass: %+v, %v", outcome, err)
	}
	if got := records.stored[large].Status; got != domain.StatusUnscannable {
		t.Errorf("the oversized file is %s, want UNSCANNABLE", got)
	}
	if got := records.stored[small].Status; got != domain.StatusReady {
		t.Errorf("the file behind it is %s, want READY in the same pass", got)
	}
}

// errorsOn answers the scanner's error reply for one file's bytes and passes the rest clean.
type errorsOn struct{ body string }

func (s errorsOn) Scan(_ context.Context, content io.Reader) (storage.Verdict, error) {
	body, _ := io.ReadAll(content)
	if string(body) == s.body {
		return storage.Verdict{}, shared.ErrUnavailable.WithDetail("dependency.scanner_unavailable").
			WithCause(storage.ErrNoVerdictForFile)
	}
	return storage.Verdict{}, nil
}

// A file the scanner answers an error for stays held, and the files behind it are still scanned:
// the scanner answered, so it is not down for all of them.
func TestTheScanJobGoesOnPastAFileTheScannerAnsweredAnErrorFor(t *testing.T) {
	records, bytesOf := newObjects(), newStore()
	odd, next := shared.ID("0192f000-0000-7000-8000-0000000000d1"), shared.ID("0192f000-0000-7000-8000-0000000000d2")
	for i, held := range []struct {
		id    shared.ID
		bytes []byte
	}{{odd, []byte("odd")}, {next, []byte("pdf")}} {
		object := domain.Object{
			ID: held.id, TenantID: tenantID, StorageKey: "media/" + tenantID.String() + "/" + held.id.String(),
			ContentType: "application/pdf", ByteSize: int64(len(held.bytes)), Usage: domain.UsageAttachment,
			Status: domain.StatusPendingScan, CreatedBy: accountID, CreatedAt: now.Add(time.Duration(i) * time.Minute),
		}
		records.stored[held.id] = object
		bytesOf.content[object.StorageKey] = held.bytes
	}
	job := ScanHeldMedia{Objects: records, Store: bytesOf, Scanner: errorsOn{body: "odd"}, UnitOfWork: &unitOfWork{}}

	outcome, err := job.Execute(t.Context(), tenantID)
	if err != nil || outcome.Settled != 1 || outcome.Held != 1 {
		t.Fatalf("one pass: %+v, %v", outcome, err)
	}
	if got := records.stored[odd].Status; got != domain.StatusPendingScan {
		t.Errorf("the file the scanner answered an error for is %s, want still held", got)
	}
	if got := records.stored[next].Status; got != domain.StatusReady {
		t.Errorf("the file behind it is %s, want READY in the same pass", got)
	}
}

// A confirmation the scanner refuses for size is refused by name and its bytes removed.
func TestAConfirmationRefusesAFileTooLargeToScan(t *testing.T) {
	handler, records, bytesOf, _, _ := confirmHarness(1 << 20)
	handler.Scanner = sizeLimited{limit: 1}
	staged(records, bytesOf, "image/png", int64(len(pngBytes)), pngBytes)

	_, err := handler.Execute(t.Context(), actor(mediaWrite), ConfirmCommand{MediaID: mintedID})
	if !domain.IsTooLargeToScan(err) || len(records.sealed) != 0 || len(bytesOf.deleted) != 1 {
		t.Fatalf("answered %v, sealed %v, removed %v", err, records.sealed, bytesOf.deleted)
	}
}
