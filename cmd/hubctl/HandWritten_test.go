// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"strings"
	"testing"
)

// handWritten is every command still written by hand, with why it is not a generated door
// (api-guidelines.md §1, `x-hubtask-cli`). "to move:" marks one that only relays its operation
// and waits to be named on it; every other reason is what the command does beyond the operation.
var handWritten = map[string]string{
	// Signing in, the local profile and the connection.
	"login":                      "prompts for the password and the code, and stores the session in the profile",
	"auth login":                 "verifies a token and stores it in the profile",
	"auth status":                "reads the local profile, no operation",
	"auth logout":                "forgets the local credential, no operation",
	"step-up":                    "prompts for the proof and prints it for another command",
	"mfa enroll":                 "shows the authenticator's secret once and how to confirm it",
	"mfa confirm":                "prompts for the code",
	"mfa disable":                "proves the person afresh with a prompt",
	"session revoke":             "one session or, with --all, every other: two operations",
	"token create":               "proves the person afresh and says the token is shown once",
	"admin elevate":              "proves the operator afresh with a prompt",
	"oauth authorize":            "walks the consent of an authorization request",
	"oauth token":                "the RFC 6749 token exchange in its form encoding",
	"sync pull":                  "keeps the device's cursor and clock in the profile",
	"sync push":                  "reads the queued changes and keeps the clock in the profile",
	"sync snapshot":              "follows the job and writes the snapshot to a file",
	"sync devices":               "forgetting a device also clears it from the profile",
	"sync-conformance":           "runs the sync protocol's conformance suite, many operations",
	"watch":                      "stays connected to the event stream",
	"search":                     "builds the search document from words and flags",
	"import":                     "reads a file, uploads it and follows the job",
	"mcp tools":                  "speaks MCP, not the REST contract",
	"mcp resources":              "speaks MCP, not the REST contract",
	"mcp prompts":                "speaks MCP, not the REST contract",
	"mcp read":                   "speaks MCP, not the REST contract",
	"mcp prompt":                 "speaks MCP, not the REST contract",
	"media upload":               "reads the file, asks for an upload address and sends the bytes",
	"calendar mint":              "says the feed's address is shown once",
	"calendar fetch":             "reads the feed's iCalendar document, not JSON",
	"view export":                "writes the export to a file",
	"audit export":               "follows the job and writes the archive to a file",
	"audit query":                "takes a day for an instant and says how the page continues",
	"audit verify":               "a broken chain fails the command",
	"audit anchor":               "reads and sets the anchoring, two operations",
	"job show":                   "follows the job with --wait",
	"backup run":                 "follows the job with --wait",
	"backup verify":              "follows the job and fails the command for a damaged archive",
	"restore inspect":            "follows the job with --wait",
	"restore run":                "follows the job with --wait and asks for the passphrase",
	"events poll":                "continues by --since, the cursor of the trigger feed",
	"trash restore":              "a hub, a collection or an entry: two operations chosen by --kind",
	"webhook add":                "repeats --event and reads a chat room's token from standard input",
	"webhook rotate-secret":      "says the secret is shown once and names the grace period",
	"webhook pause":              "a fixed state of the change, never DISABLED",
	"webhook resume":             "a fixed state of the change",
	"rule add":                   "repeats --action as KIND or KIND:json",
	"rule test":                  "builds the sample event from flags and says nothing was done",
	"rule rotate-token":          "says the token is shown once",
	"jumble intake":              "says the intake's token is shown once",
	"jumble show":                "leaves the raw body out of the table",
	"container create":           "builds the container from flags and a preset",
	"container structure":        "draws the levels and their settings as a table",
	"container emergency-access": "proves the person afresh with a prompt",
	"workspace structure":        "draws the levels and their settings as a table",
	"preset ls":                  "draws each preset's levels and views",
	"quota show":                 "draws each quota with its use and approach",
	"module ls":                  "draws each module's state and who decided it",
	"module on":                  "a fixed state of the module's switch",
	"module off":                 "a fixed state of the module's switch",
	"module inherit":             "a fixed state of the module's switch",
	"item ls":                    "draws what the collection hides of each entry (UC-WRK-24)",
	"item create":                "builds the entry from flags: due, recurrence, fields",
	"item move":                  "names the place by two identifiers and a position",
	"due set":                    "builds the due trio from a day or an instant and a zone",
	"recur set":                  "builds the series from flags",
	"remind ls":                  "draws each reminder's offset",
	"remind add":                 "builds the offset from --at or --to",
	"comment ls":                 "a deleted comment reads as deleted, a multi-line body stays on its row",
	"comment add":                "answers by --reply-to",
	"field define":               "builds the definition from flags by its kind",
	"field set":                  "types the value by the field's kind, or clears it",
	"template ls":                "counts each template's nodes",
	"template create":            "reads the tree from a file",
	"template instantiate":       "builds the stamping from flags",
	"view create":                "builds the query from flags",
	"type ls":                    "draws each type's capabilities in one column",
	"type create":                "builds the type's capabilities from flags",
	"type set":                   "builds the change from flags",
	"type archive":               "a fixed member of the change",
	"type unarchive":             "a fixed member of the change",
	"type assign":                "confirms the values the change drops",
	"type form get":              "reads the type's form from the list",
	"hold place":                 "builds the scope from --scope and --id",
	"retention ls":               "draws a rule's failure beside it",
	"retention add":              "builds the rule from flags",
	"retention set":              "builds the change from flags",
	"retention preview":          "draws what would go and what stops it, in a stable order",
	"suggestion ls":              "to move: its page is `items`, not `data`",
	"suggestion ask":             "to move: relays the operation",
	"suggestion accept":          "repeats --override name=value",
	"jumble submit":              "to move: repeats --attachment",
	"people ls":                  "names the scope by --scope",
	"people invite":              "to move: proves the person afresh",
	"people grant":               "to move: proves the person afresh",
	"people add-managed":         "says the start password is shown once",
	"dsr extend":                 "checks the three days before the call",
	"dsr create":                 "builds the case from flags",
	"dsr ls":                     "draws both deadlines and what a hold kept",
	"dsr preview":                "draws what each hold would keep",
	"dsr start":                  "a fixed status of the change",
	"dsr complete":               "a fixed status of the change",
	"dsr reject":                 "a fixed status of the change, a reason demanded",
	"config schema":              "prints the schema embedded in the binary",
	"config export":              "prints the document whole as JSON, never a table",
	"config explain":             "takes the key path positionally",
	"config validate":            "reads the document from a file",
	"config plan":                "reads the document from a file",
	"config apply":               "plans, shows the plan and applies it with --yes",
	"config history":             "draws the versions as a table",
	"config rollback":            "plans the version and applies it with --yes",
	"config proposals":           "draws the proposals as a table",
	"config approve":             "a fixed decision",
	"config decline":             "a fixed decision",
	"config policy":              "takes the policy positionally",
	"config templates":           "draws the templates as a table",
	"config publish":             "reads the blocks from a file",
	"config take":                "plans the template's upgrade by key paths",
	"config sandbox":             "shows, changes, resets or discards the sandbox: several operations",
	"config promote":             "plans and applies the sandbox with --yes",
	"ai config":                  "shows or sets the provider: two operations",
	"ai config-set":              "builds the provider from flags and reads its key from standard input",
	"ai source":                  "one member of the offering document, shown or set",
	"ai opt-out":                 "one member of the offering document, shown or set",
	"ai arrival-proposals":       "one member of the offering document, shown or set",
	"ai budget":                  "shows or sets the budget: two operations",
	"oauth client":               "to move: a group of its own (add, ls, rm)",
	"oauth grant":                "to move: a group of its own (ls, revoke)",
	"backup target":              "to move: a group of its own (add, ls, test, rm)",
	"backup schedule":            "to move: a group of its own (ls, set, rm)",
	"admin tenant":               "to move: a group of its own; open-password and replace-holds prompt",
	"admin plan":                 "to move: a group of its own (ls, show, create, set, clear, rm)",
	"admin presets":              "to move: a group of its own (ls, create, set, rm)",
	"admin events":               "to move: relays the operation",
	"admin subscription":         "to move: a group of its own; add and rotate-secret show a secret once",
	"admin usage":                "draws the installation's use as a table",
	"admin settings":             "shows, sets or clears one section of the instance settings",
	"admin operator":             "to move: a group of its own (ls, add, rm)",
	"admin provider":             "to move: a group of its own (ls, add, rm)",
	"admin ai":                   "shows or sets the installation's AI: several operations",
	"admin target":               "to move: a group of its own (ls, add, test, rm)",
	"admin encryption":           "to move: a group of its own (show, reseal)",
}

// Every hand-written command says why it is not generated, and the table names nothing that is
// gone - so a new hand-written command, or one moved onto its operation, changes this table.
func TestEveryHandWrittenCommandSaysWhy(t *testing.T) {
	saved := generatedCommands
	generatedCommands = nil
	tree := groups()
	generatedCommands = saved

	generated := map[string]bool{}
	for _, d := range generatedCommands {
		generated[d.Group+" "+d.Verb] = true
	}
	present := map[string]bool{}
	for _, g := range tree {
		names := []string{g.name}
		if g.run == nil {
			names = names[:0]
			for _, c := range g.commands {
				names = append(names, g.name+" "+c.name)
			}
		}
		for _, name := range names {
			present[name] = true
			reason := handWritten[name]
			if strings.TrimSpace(reason) == "" {
				t.Errorf("hubctl %s is hand-written and says not why: name it with x-hubtask-cli on its operation, or add it to handWritten", name)
			}
			if generated[name] && strings.HasPrefix(reason, "to move:") {
				t.Errorf("hubctl %s is generated as well: remove the hand-written one", name)
			}
		}
	}
	for name := range handWritten {
		if !present[name] {
			t.Errorf("handWritten names hubctl %s, which is no hand-written command", name)
		}
	}
}
