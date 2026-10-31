// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"strings"
	"testing"
)

const aliasID = "0195f0a0-0000-7000-8000-0000000000ab"
const aliasOtherID = "0195f0a0-0000-7000-8000-0000000000ac"

// A flag renamed to the contract's name keeps its old name, so a script written against the
// hand-written command still runs: a renamed value, a switch standing for a value, and a flag
// that became a positional argument.
func TestAFormerFlagNameStillWorks(t *testing.T) {
	cases := []struct {
		args []string
		path string
		want []string
	}{
		{[]string{"token", "ls", "--account", aliasID}, "/tokens", []string{"account_id=" + aliasID}},
		{[]string{"rule", "ls", "--disabled"}, "/automation/rules", []string{"enabled=false"}},
		{[]string{"container", "ls", "--parent=" + aliasID}, "/containers", []string{"parent_id=" + aliasID}},
		{[]string{"item", "assign", aliasID, "--account", aliasOtherID}, "/items/" + aliasID + ":assign", []string{`"account_id":"` + aliasOtherID + `"`}},
		{[]string{"item", "complete", aliasID, "--cascade"}, "/items/" + aliasID + ":complete", []string{`"cascade_children":true`}},
		{[]string{"media", "attach", aliasID, "--media", aliasOtherID}, "/items/" + aliasID + "/attachments/" + aliasOtherID, nil},
		{[]string{"backup", "ls", "--target", aliasID}, "/backup-targets/" + aliasID + "/backups", nil},
	}
	for _, c := range cases {
		stub := serveJSON(t, http.StatusOK, `{"data":[],"page":{"next_cursor":null}}`)
		code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", c.args...)
		if code != exitOK {
			t.Errorf("%v: exit %d: %s", c.args, code, errOut)
			continue
		}
		if !strings.HasSuffix(stub.request.URL.Path, c.path) {
			t.Errorf("%v reached %s", c.args, stub.request.URL.Path)
		}
		sent := stub.request.URL.RawQuery + stub.body
		for _, fragment := range c.want {
			if !strings.Contains(sent, fragment) {
				t.Errorf("%v sent %q, missing %s", c.args, sent, fragment)
			}
		}
	}
}

// Every former name points at a flag its command declares today, so an alias cannot outlive the
// name it stands for.
func TestEveryFormerFlagNameNamesAFlagOfItsCommand(t *testing.T) {
	declared := map[string]map[string]bool{}
	for _, d := range generatedCommands {
		names := map[string]bool{"expect-version": d.IfMatch}
		for _, p := range append(append(append([]doorParam{}, d.PathParams...), d.Query...), d.Body...) {
			names[flagName(p.Name)] = true
		}
		declared[d.Group+" "+d.Verb] = names
	}
	for command, aliases := range flagAliases {
		names, ok := declared[command]
		if !ok {
			t.Errorf("%s is no generated command", command)
			continue
		}
		for _, a := range aliases {
			if a.Position < 0 && !names[a.New] {
				t.Errorf("%s: --%s stands for --%s, which it does not declare", command, a.Old, a.New)
			}
			if names[a.Old] {
				t.Errorf("%s: --%s is a flag of its own, not a former name", command, a.Old)
			}
		}
	}
}
