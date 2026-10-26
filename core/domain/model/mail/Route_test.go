// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package mail

import (
	"reflect"
	"testing"
)

var (
	gmail    = Door{Mailbox: "familie@gmail.com"}
	operator = Door{Domain: "in.hubtask.app", Workspace: "familie-meier"}
)

func TestTheSubjectRoutesThroughEveryDoor(t *testing.T) {
	cases := []struct {
		name       string
		subject    string
		recipients []string
		door       Door
		keys       []Key
		handles    []string
	}{
		{"a bracketed key", "Re: [WEB-42] Logo", []string{"familie@gmail.com"}, gmail,
			[]Key{{"web", 42}}, nil},
		{"a bare key", "Zum Vorgang WEB-42, bitte", nil, gmail, []Key{{"web", 42}}, nil},
		{"a handle", "Rechnung #Büro", []string{"familie-meier@in.hubtask.app"}, operator,
			nil, []string{"buro", "buero"}},
		{"two handles in order", "#einkauf #metro", nil, gmail, nil, []string{"einkauf", "metro"}},
		{"a plus handle on the mailbox", "Milch", []string{"Familie <familie+einkauf@gmail.com>"}, gmail,
			nil, []string{"einkauf"}},
		{"a plus key on the mailbox", "Antwort", []string{"familie+web-42@gmail.com"}, gmail,
			[]Key{{"web", 42}}, nil},
		{"a label on the operator's domain", "x", []string{"einkauf.familie-meier@in.hubtask.app"}, operator,
			nil, []string{"einkauf"}},
		{"a plus on the operator's domain", "x", []string{"familie-meier+web-7@in.hubtask.app"}, operator,
			[]Key{{"web", 7}}, nil},
		{"the address before the subject", "[WEB-1]", []string{"familie+web-2@gmail.com"}, gmail,
			[]Key{{"web", 2}, {"web", 1}}, nil},
		{"no word inside another", "mail@WEB-42 a#einkauf x-WEB-3", nil, gmail, nil, nil},
		{"another mailbox's plus is not ours", "x", []string{"other+einkauf@gmail.com"}, gmail, nil, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			route := RouteOf(c.subject, c.recipients, c.door)
			if !reflect.DeepEqual(route.Keys, c.keys) || !reflect.DeepEqual(route.Handles, c.handles) {
				t.Fatalf("RouteOf(%q, %v) = %+v, want keys %v handles %v", c.subject, c.recipients, route, c.keys, c.handles)
			}
		})
	}
}

func TestTheWorkspaceIsReadOffTheOperatorsDomain(t *testing.T) {
	for address, want := range map[string]string{
		"familie-meier@in.hubtask.app":         "familie-meier",
		"einkauf.familie-meier@in.hubtask.app": "familie-meier",
		"familie-meier+web-42@IN.hubtask.app":  "familie-meier",
	} {
		if got, ok := WorkspaceOf(address, "in.hubtask.app"); !ok || got != want {
			t.Errorf("WorkspaceOf(%q) = %q, %v", address, got, ok)
		}
	}
	if _, ok := WorkspaceOf("someone@gmail.com", "in.hubtask.app"); ok {
		t.Error("an address on another domain names no workspace")
	}
	if _, ok := WorkspaceOf("familie-meier@in.hubtask.app", ""); ok {
		t.Error("without a receiving domain no address names a workspace")
	}
}

func TestTheTitleLosesTheRoutingAndThePrefixes(t *testing.T) {
	for subject, want := range map[string]string{
		"WG: Rechnung #büro":           "Rechnung",
		"Re: AW: [WEB-42] Logo":        "Logo",
		"Milch und Eier":               "Milch und Eier",
		"#einkauf":                     "",
		"Fwd: Angebot für #web, bitte": "Angebot für , bitte",
	} {
		if got := Title(subject); got != want {
			t.Errorf("Title(%q) = %q, want %q", subject, got, want)
		}
	}
}

func TestAKeyIsWrittenInCapitals(t *testing.T) {
	if got := (Key{Handle: "einkauf", Number: 42}).String(); got != "EINKAUF-42" {
		t.Fatalf("got %q", got)
	}
}
