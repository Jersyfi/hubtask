// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package filelink

import (
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A link's store is told by its address, a name taken from it, and an address a person could not
// open safely is refused (UC-INT-14 check 1).
func TestCheckTellsTheStoreAndRefusesWhatCannotBeOpened(t *testing.T) {
	cases := []struct {
		address  string
		provider Provider
		name     string
		wantAddr string
		wantProv Provider
		wantName string
		code     string
	}{
		{address: "https://cloud.example.org/s/aBcD1234", wantProv: Nextcloud, wantName: "aBcD1234",
			wantAddr: "https://cloud.example.org/s/aBcD1234"},
		{address: "https://cloud.example.org/index.php/f/42", wantProv: Nextcloud, wantName: "42"},
		{address: "https://cloud.example.org/remote.php/dav/files/ada/Specs/Login%20flow.pdf", wantProv: Nextcloud,
			wantName: "Login flow.pdf"},
		{address: "https://contoso.sharepoint.com/sites/web/Shared%20Documents/Plan.docx", wantProv: SharePoint,
			wantName: "Plan.docx"},
		{address: "https://contoso-my.sharepoint.com/personal/ada/Documents/Notes.xlsx", wantProv: OneDrive,
			wantName: "Notes.xlsx"},
		{address: "https://1drv.ms/w/s!Abc", wantProv: OneDrive},
		{address: "https://files.example.org/dav/Team/", wantProv: WebDAV, wantName: "Team"},
		{address: "davs://files.example.org/dav/Team/a.txt", wantProv: WebDAV, wantAddr: "https://files.example.org/dav/Team/a.txt"},
		{address: "https://files.example.org/x", provider: Nextcloud, name: " Spec ", wantProv: Nextcloud, wantName: "Spec"},
		{address: "https://files.example.org", wantProv: WebDAV, wantName: "files.example.org"},
		{address: "", code: "filelink.address_required"},
		{address: "http://files.example.org/a", code: "filelink.address_not_https"},
		{address: "javascript:alert(1)", code: "filelink.address_invalid"},
		{address: "https://ada:secret@files.example.org/a", code: "filelink.address_has_credentials"},
		{address: "https://files.example.org/a", provider: "DROPBOX", code: "filelink.provider_unknown"},
		{address: "https://files.example.org/" + strings.Repeat("a", MaxAddressBytes), code: "filelink.address_too_long"},
		{address: "https://files.example.org/a", name: strings.Repeat("n", MaxNameRunes+1), code: "filelink.name_too_long"},
	}
	for _, c := range cases {
		addr, prov, name, err := Check(c.address, c.provider, c.name)
		if c.code != "" {
			var e *shared.Error
			if !errors.As(err, &e) || e.DetailCode != c.code {
				t.Errorf("%q: error %v, want %s", c.address, err, c.code)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", c.address, err)
			continue
		}
		if prov != c.wantProv {
			t.Errorf("%q: provider %s, want %s", c.address, prov, c.wantProv)
		}
		if c.wantName != "" && name != c.wantName {
			t.Errorf("%q: name %q, want %q", c.address, name, c.wantName)
		}
		if c.wantAddr != "" && addr != c.wantAddr {
			t.Errorf("%q: address %q, want %q", c.address, addr, c.wantAddr)
		}
	}
}
