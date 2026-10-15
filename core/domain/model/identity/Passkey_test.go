// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func TestAPasskeysNameIsTrimmedAndBounded(t *testing.T) {
	for _, c := range []struct {
		raw, want string
		ok        bool
	}{
		{"iPhone", "iPhone", true},
		{"  Laptop at work \t", "Laptop at work", true},
		{"Schlüssel in der Schublade", "Schlüssel in der Schublade", true},
		{"\U0001F469\u200d\U0001F4BB laptop", "\U0001F469\u200d\U0001F4BB laptop", true},
		{strings.Repeat("ä", MaxPasskeyNameLength), strings.Repeat("ä", MaxPasskeyNameLength), true},
		{strings.Repeat("ä", MaxPasskeyNameLength+1), "", false},
		{"", "", false},
		{"   ", "", false},
		{"key\nline", "", false},
		{"key\x00", "", false},
		{"key\u202eevil", "", false},
		{"key\u2066isolated\u2069", "", false},
		{"\xff\xfe", "", false},
	} {
		got, err := PasskeyName(c.raw)
		if c.ok {
			if err != nil || got != c.want {
				t.Errorf("%q: got %q, %v", c.raw, got, err)
			}
			continue
		}
		var domainErr *shared.Error
		if !errors.As(err, &domainErr) || domainErr.DetailCode != "auth.passkey_name_invalid" {
			t.Errorf("%q: accepted as %q (%v)", c.raw, got, err)
		}
	}
}

func TestAWorkspacesRelyingPartyIsItsHost(t *testing.T) {
	for _, c := range []struct {
		name, host, scheme, port string
		rpID, origin             string
		ok                       bool
	}{
		{"single mode", "tasks.example.org", "https", "", "tasks.example.org", "https://tasks.example.org", true},
		{"multi mode, the workspace's subdomain", CanonicalHostOf("acme", "hubtask.example", true), "https", "",
			"acme.hubtask.example", "https://acme.hubtask.example", true},
		{"the default port is not written", "tasks.example.org", "https", "443", "tasks.example.org", "https://tasks.example.org", true},
		{"another port is", "tasks.example.org", "https", "8443", "tasks.example.org", "https://tasks.example.org:8443", true},
		{"a development server", "localhost", "http", "8080", "localhost", "http://localhost:8080", true},
		{"http's default port", "localhost", "http", "80", "localhost", "http://localhost", true},
		{"case and a trailing dot", " Tasks.Example.ORG. ", "HTTPS", "", "tasks.example.org", "https://tasks.example.org", true},
		{"no host", "", "https", "", "", "", false},
		{"no scheme", "tasks.example.org", "", "", "", "", false},
		{"another scheme", "tasks.example.org", "ftp", "", "", "", false},
	} {
		rpID, origin, ok := PasskeyRelyingParty(c.host, c.scheme, c.port)
		if rpID != c.rpID || origin != c.origin || ok != c.ok {
			t.Errorf("%s: (%q, %q, %v), want (%q, %q, %v)", c.name, rpID, origin, ok, c.rpID, c.origin, c.ok)
		}
	}
	// Multi mode never answers the parent: a passkey made in one workspace is not one of another's.
	if rpID, _, _ := PasskeyRelyingParty(CanonicalHostOf("acme", "hubtask.example", true), "https", ""); rpID == "hubtask.example" {
		t.Error("the RP ID is the registrable parent")
	}
}
