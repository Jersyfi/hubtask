// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import "strings"

// flagAlias is a flag's former name, from before its command was generated and took the
// contract's member names: an existing script keeps working (accepted.md, third answers 7).
// Accepted, never advertised - the help names the contract's flag only.
type flagAlias struct {
	Old, New string
	// Value, where set, makes the old switch stand for --New=Value (`--off` is `--enabled=false`).
	Value string
	// Position, from 0, is the path argument a flag became (`backup ls --target <id>` is
	// `backup ls <id>`); -1 for a flag.
	Position int
}

func renamed(old, now string) flagAlias { return flagAlias{Old: old, New: now, Position: -1} }
func standsFor(old, now, value string) flagAlias {
	return flagAlias{Old: old, New: now, Value: value, Position: -1}
}
func becamePositional(old string, at int) flagAlias { return flagAlias{Old: old, Position: at} }

// flagAliases holds the renames of the generated commands that replaced hand-written ones. A
// former name whose meaning changed is not here: `jumble submit --body` is the request's file now,
// and a text there would be read as a path.
var flagAliases = map[string][]flagAlias{
	"rule ls":                {standsFor("disabled", "enabled", "false")},
	"rule runs":              {renamed("rule", "rule-id")},
	"token ls":               {renamed("account", "account-id")},
	"service-account create": {renamed("name", "display-name")},
	"view ls":                {renamed("container", "container-id")},
	"backup ls":              {becamePositional("target", 0)},
	"container ls":           {renamed("parent", "parent-id")},
	"item complete":          {renamed("cascade", "cascade-children")},
	"item assign":            {renamed("account", "account-id")},
	"media attach":           {becamePositional("media", 1)},
	"field ls":               {renamed("collection", "collection-id")},
	"jumble convert":         {renamed("bucket", "bucket-id"), renamed("collection", "collection-id")},
	"jumble submit":          {renamed("subject", "raw-subject"), renamed("attachment", "attachments")},
	"admin tenant delete":    {renamed("confirm", "confirmation")},
	"admin tenant find":      {renamed("reference", "external-reference")},
	"admin provider add": {
		renamed("name", "display-name"), renamed("admits", "provisioning"),
		renamed("directory", "allowed-directories"), renamed("domain", "allowed-email-domains"),
	},
	"admin plan create":    {renamed("leave-out", "left-out")},
	"admin presets create": {renamed("from", "preset"), renamed("default", "is-default"), renamed("lock", "locks")},
	"admin presets set": {
		renamed("from", "preset"), renamed("default", "is-default"), renamed("lock", "locks"),
		renamed("expected-version", "expect-version"),
	},
	"backup schedule set": {
		standsFor("on", "enabled", "true"), standsFor("off", "enabled", "false"),
		standsFor("trial", "trial-restore", "true"), standsFor("no-trial", "trial-restore", "false"),
	},
	"suggestion ls": {renamed("target", "target-id")},
}

// withFormerNames rewrites a command's arguments from former flag names to today's, before
// anything is parsed.
func withFormerNames(command string, args []string) []string {
	aliases := flagAliases[command]
	if len(aliases) == 0 {
		return args
	}
	out := make([]string, 0, len(args))
	moved := map[int]string{}
	for i := 0; i < len(args); i++ {
		name, value, hasValue := splitFlag(args[i])
		alias, found := formerName(aliases, name)
		switch {
		case !found:
			out = append(out, args[i])
		case alias.Position >= 0:
			if !hasValue && i+1 < len(args) {
				i++
				value = args[i]
			}
			moved[alias.Position] = value
		case alias.Value != "":
			// A bare switch, or one given true; `--off=false` meant nothing and says nothing.
			if !hasValue || value == "true" {
				out = append(out, "--"+alias.New+"="+alias.Value)
			}
		case hasValue:
			out = append(out, "--"+alias.New+"="+value)
		default:
			out = append(out, "--"+alias.New)
		}
	}
	if len(moved) == 0 {
		return out
	}
	leading := 0
	for leading < len(out) && !strings.HasPrefix(out[leading], "-") {
		leading++
	}
	positional := append([]string{}, out[:leading]...)
	for at := 0; at < leading+len(moved); at++ {
		if value, ok := moved[at]; ok {
			positional = append(positional[:min(at, len(positional))],
				append([]string{value}, positional[min(at, len(positional)):]...)...)
		}
	}
	return append(positional, out[leading:]...)
}

// splitFlag reads `--name`, `-name` or `--name=value`; anything else is no flag.
func splitFlag(arg string) (name, value string, hasValue bool) {
	if !strings.HasPrefix(arg, "-") || arg == "-" || arg == "--" {
		return "", "", false
	}
	name = strings.TrimLeft(arg, "-")
	if at := strings.IndexByte(name, '='); at >= 0 {
		return name[:at], name[at+1:], true
	}
	return name, "", false
}

func formerName(aliases []flagAlias, name string) (flagAlias, bool) {
	if name == "" {
		return flagAlias{}, false
	}
	for _, a := range aliases {
		if a.Old == name {
			return a, true
		}
	}
	return flagAlias{}, false
}
