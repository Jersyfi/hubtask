// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package filelink

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func codeOf(err error) string {
	var e *shared.Error
	if errors.As(err, &e) {
		return e.DetailCode
	}
	return ""
}

// A Nextcloud's address is read as a person types or pastes it, and one the product could not
// sign in to safely is refused (UC-INT-17 check 1).
func TestCheckServerReadsANextcloudsAddress(t *testing.T) {
	cases := []struct{ in, want, code string }{
		{in: "cloud.example.org", want: "https://cloud.example.org"},
		{in: "https://Cloud.Example.org/", want: "https://cloud.example.org"},
		{in: "https://example.org/nextcloud/index.php/apps/files/?dir=/", want: "https://example.org/nextcloud"},
		{in: "https://cloud.example.org/remote.php/dav/files/ada", want: "https://cloud.example.org"},
		{in: "", code: "filestore.server_required"},
		{in: "http://cloud.example.org", code: "filestore.server_not_https"},
		{in: "https://ada:pw@cloud.example.org", code: "filestore.server_invalid"},
		{in: "https://" + strings.Repeat("a", MaxServerBytes), code: "filestore.server_invalid"},
	}
	for _, c := range cases {
		got, err := CheckServer(c.in)
		if c.code != "" {
			if codeOf(err) != c.code {
				t.Errorf("%q: %v, want %s", c.in, err, c.code)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%q: %q %v, want %q", c.in, got, err, c.want)
		}
	}
}

func TestNewConnectionKeepsWhatTheStoreAnswered(t *testing.T) {
	now := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	c, err := NewConnection("c", "t", "a", ConnectNextcloud, "https://cloud.example.org/index.php", " ada ", now)
	if err != nil || c.Server != "https://cloud.example.org" || c.Login != "ada" {
		t.Fatalf("nextcloud: %+v %v", c, err)
	}
	if _, err := NewConnection("c", "t", "a", ConnectNextcloud, "https://cloud.example.org", "", now); codeOf(err) != "filestore.login_required" {
		t.Fatalf("no login: %v", err)
	}
	m, err := NewConnection("c", "t", "a", ConnectMicrosoft, "https://ignored.example.org", "ada@example.org", now)
	if err != nil || m.Server != "" {
		t.Fatalf("microsoft: %+v %v", m, err)
	}
	if _, err := ParseConnectionKind("dropbox"); codeOf(err) != "filestore.kind_unknown" {
		t.Fatalf("kind: %v", err)
	}
	for _, scope := range ConnectMicrosoft.Scopes() {
		if strings.Contains(scope, "Mail") || strings.Contains(scope, "ReadWrite") {
			t.Fatalf("a connection asks to read files alone, got %s", scope)
		}
	}
}

// A folder handed back by a client never steps out of the person's own files.
func TestCheckFolderRefRefusesAStepOut(t *testing.T) {
	for _, ok := range []string{"", "/Specs/2026", "drive:b!x:01ABC"} {
		if _, err := CheckFolderRef(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"/Specs/../../other", "..", "a\x00b", strings.Repeat("a", MaxFolderRefSize+1)} {
		if _, err := CheckFolderRef(bad); codeOf(err) != "filestore.folder_invalid" {
			t.Errorf("%q: %v", bad, err)
		}
	}
}
