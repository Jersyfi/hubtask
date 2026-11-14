// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package media_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/media"
)

func TestTheDocumentServerChoiceTakesAnHttpsOriginOrNone(t *testing.T) {
	cases := []struct {
		kind    media.DocumentServer
		address string
		want    string
		isValid bool
	}{
		{media.DocumentServerNone, "", "", true},
		{media.DocumentServerCollabora, "https://office.example.org/", "https://office.example.org", true},
		{media.DocumentServerEuroOffice, "https://docs.example.org:8443", "https://docs.example.org:8443", true},
		{media.DocumentServerOnlyOffice, "http://docs.example.org", "", false},
		{media.DocumentServerOnlyOffice, "https://user:pw@docs.example.org", "", false},
		{media.DocumentServerCollabora, "https://office.example.org/?token=x", "", false},
		{media.DocumentServerCollabora, "office.example.org", "", false},
		{media.DocumentServer("WORDPAD"), "https://office.example.org", "", false},
	}
	for _, each := range cases {
		choice, err := media.NewDocumentServerChoice(each.kind, each.address)
		if (err == nil) != each.isValid {
			t.Errorf("%s %q: err = %v, want valid %v", each.kind, each.address, err, each.isValid)
			continue
		}
		if each.isValid && choice.URL != each.want {
			t.Errorf("%s %q: URL = %q, want %q", each.kind, each.address, choice.URL, each.want)
		}
	}
	none, _ := media.NewDocumentServerChoice(media.DocumentServerNone, "")
	if none.IsOn() {
		t.Error("NONE sends office files to a document server")
	}
}

func TestOfficeFilesAreKnownByTypeOrEnding(t *testing.T) {
	office := [][2]string{
		{"application/vnd.openxmlformats-officedocument.wordprocessingml.document", "x"},
		{"application/zip", "Lastenheft.docx"},
		{"application/octet-stream", "Testmatrix.XLSX"},
		{"application/vnd.oasis.opendocument.presentation", "deck.odp"},
	}
	for _, each := range office {
		if !media.IsOfficeFile(each[0], each[1]) {
			t.Errorf("%v is not an office file", each)
		}
	}
	for _, each := range [][2]string{{"application/pdf", "a.pdf"}, {"application/zip", "logs.zip"}, {"text/csv", "matrix.csv"}} {
		if media.IsOfficeFile(each[0], each[1]) {
			t.Errorf("%v is an office file", each)
		}
	}
}

// The WOPI lock protocol: one lock at a time, renewed by its holder, released only by it, and a
// conflict that names the lock that holds.
func TestTheLockProtocol(t *testing.T) {
	now := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	held := &media.Lock{ID: "A", ExpiresAt: now.Add(10 * time.Minute)}
	expired := &media.Lock{ID: "A", ExpiresAt: now.Add(-time.Second)}

	conflict := func(err error) string {
		var c media.LockConflict
		if !errors.As(err, &c) {
			return "<no conflict>"
		}
		return c.Current
	}

	if lock, err := media.TakeLock(nil, "A", now, false); err != nil || lock.ExpiresAt != now.Add(media.LockDuration) {
		t.Errorf("an unlocked file is not locked: %v %v", lock, err)
	}
	if lock, err := media.TakeLock(held, "A", now, false); err != nil || lock.ExpiresAt != now.Add(media.LockDuration) {
		t.Errorf("the holder does not renew its lock: %v", err)
	}
	if _, err := media.TakeLock(held, "B", now, false); conflict(err) != "A" {
		t.Errorf("another lock is taken over a held one: %v", err)
	}
	if _, err := media.TakeLock(expired, "B", now, false); err != nil {
		t.Errorf("an expired lock still holds: %v", err)
	}
	if _, err := media.TakeLock(nil, "A", now, true); conflict(err) != "" {
		t.Errorf("a refresh of no lock succeeds: %v", err)
	}
	if _, err := media.Relock(held, "A", "C", now); err != nil {
		t.Errorf("the holder cannot relock: %v", err)
	}
	if _, err := media.Relock(held, "B", "C", now); conflict(err) != "A" {
		t.Errorf("a stranger relocks: %v", err)
	}
	if err := media.Release(held, "A", now); err != nil {
		t.Errorf("the holder cannot unlock: %v", err)
	}
	if err := media.Release(held, "B", now); conflict(err) != "A" {
		t.Errorf("a stranger unlocks: %v", err)
	}
	if err := media.MayPut(held, "A", 100, now); err != nil {
		t.Errorf("the holder cannot save: %v", err)
	}
	if err := media.MayPut(held, "B", 100, now); conflict(err) != "A" {
		t.Errorf("a stranger saves under another's lock: %v", err)
	}
	if err := media.MayPut(nil, "", 100, now); conflict(err) != "" {
		t.Errorf("an unlocked file with content is overwritten: %v", err)
	}
	if err := media.MayPut(nil, "", 0, now); err != nil {
		t.Errorf("an empty unlocked file cannot be written: %v", err)
	}
}
