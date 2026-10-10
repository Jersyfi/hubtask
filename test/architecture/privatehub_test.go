// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"os"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"

	usecases "github.com/Jersyfi/hubtask/core/application/catalogue"
)

// The reasons a reader may stay out of the private-hub table (D14 part 3). Each names why the
// reader cannot show a private hub to somebody outside it.
const (
	// asksOnItsPath: the reader asks the authoriser (Authorize, ReachInto, Permitted) on the path
	// of the entry, container, view or template it reads before it answers anything, and privacy is
	// in that resolution - the table's rows prove it for the same calls.
	asksOnItsPath = "asks the authoriser on the path of what it reads"
	// noHub: the reader answers accounts, sessions, credentials, providers or the workspace's
	// own settings, none of which lies in a hub.
	noHub = "reads nothing that lies in a hub"
	// controllersInstrument: ADR-0073 §5 - a legal hold or a retention rule applies to a private
	// hub as everywhere; the reader shows identifiers and counts, never a hub's name or content.
	controllersInstrument = "the controller's instrument (ADR-0073 §5): identifiers and counts only"
	// ownSubscriptions: a subscription is its creator's, deliveries exist only for events its
	// creator reaches (D5), and a delivery answers its status and the event's identifier only.
	ownSubscriptions = "a subscription delivers only what its creator reaches; no event content"
	// filteredPoll: the poll leaves out what the caller does not reach (D5).
	filteredPoll = "leaves out every event the caller does not reach"
)

// privateHubExemptions are the readers of the six packages UC-ID-16 check 2 names that are not rows
// of the integration table, each with its reason.
var privateHubExemptions = map[string]string{
	"AiTranslate":                  asksOnItsPath, // reads through GetWorkItem
	"ExportView":                   asksOnItsPath,
	"GetRecurrence":                asksOnItsPath,
	"GetSavedView":                 asksOnItsPath,
	"GetTemplate":                  asksOnItsPath,
	"ListActivity":                 asksOnItsPath,
	"ListBuckets":                  asksOnItsPath,
	"ListCalendarFeeds":            asksOnItsPath,
	"ListCustomFields":             asksOnItsPath,
	"ListLabels":                   asksOnItsPath,
	"ListReminders":                asksOnItsPath,
	"ListSavedViews":               asksOnItsPath,
	"ListTemplates":                asksOnItsPath,
	"QueryItems":                   asksOnItsPath,
	"GetMedia":                     asksOnItsPath,
	"ListAttachments":              asksOnItsPath,
	"ListSuggestions":              asksOnItsPath,
	"ListMemberships":              asksOnItsPath,
	"ListLegalHolds":               controllersInstrument,
	"ListRetentionPolicies":        controllersInstrument,
	"PreviewRetentionPolicy":       controllersInstrument,
	"GetWebhookSubscription":       ownSubscriptions,
	"ListWebhookDeliveries":        ownSubscriptions,
	"ListWebhookSubscriptions":     ownSubscriptions,
	"PollTriggerEvents":            filteredPoll,
	"ReadAiProvider":               noHub,
	"CheckPassword":                noHub,
	"CountAccountsWithoutProvider": noHub,
	"GetAccount":                   noHub,
	"GetGroup":                     noHub,
	"GetOwnAccount":                noHub,
	"GetSignInRules":               noHub,
	"ListAccessTokens":             noHub,
	"ListGroups":                   noHub,
	"ListIdentityProviderPresets":  noHub,
	"ListIdentityProviders":        noHub,
	"ListNotificationPreferences":  noHub,
	"ListOauthClients":             noHub,
	"ListOauthGrants":              noHub,
	"ListServiceAccounts":          noHub,
	"ListSessions":                 noHub,
	"ReadIdentityProvider":         noHub,
	"ReadOauthClient":              noHub,
	"ReadWorkspace":                noHub,
}

// readerPackages are the packages whose readers UC-ID-16 check 2 is about.
var readerPackages = []string{"work", "media", "suggestion", "lifecycle", "identity", "integration"}

// TestEveryReaderIsInThePrivateHubTable holds the catalogue to the reader table of
// test/integration/private_hub_readers_test.go (D14 part 3): a reading use case added to one of the
// six packages is a row of the table, which runs it against PostgreSQL as a workspace owner outside
// a private hub, or an exemption here with its reason. Either way somebody decided.
func TestEveryReaderIsInThePrivateHubTable(t *testing.T) {
	source, err := os.ReadFile("../integration/private_hub_readers_test.go")
	if err != nil {
		t.Fatalf("reading the table: %v", err)
	}
	block := regexp.MustCompile(`(?s)var PrivateHubReaders = \[\]string\{(.*?)\n\}`).FindSubmatch(source)
	if block == nil {
		t.Fatal("the table PrivateHubReaders is not where this test reads it")
	}
	table := map[string]bool{}
	for _, name := range regexp.MustCompile(`work\.(\w+)Name`).FindAllSubmatch(block[1], -1) {
		table[string(name[1])] = true
	}

	served := map[string]bool{}
	for _, descriptor := range usecases.Descriptors() {
		if !descriptor.ReadOnly || !inReaderPackage(descriptor.Handler) {
			continue
		}
		served[descriptor.Name] = true
		_, exempt := privateHubExemptions[descriptor.Name]
		switch {
		case table[descriptor.Name] && exempt:
			t.Errorf("%s is a row of the table and exempt - one or the other", descriptor.Name)
		case !table[descriptor.Name] && !exempt:
			t.Errorf("%s reads in a package UC-ID-16 check 2 names and is neither a row of the "+
				"private-hub reader table nor exempt with a reason (D14)", descriptor.Name)
		}
	}
	for name := range privateHubExemptions {
		if !served[name] {
			t.Errorf("the exemption %s names no reader of the six packages", name)
		}
	}
	for name := range table {
		if !served[name] {
			t.Errorf("the table's row %s names no reader of the six packages", name)
		}
	}
}

func inReaderPackage(handler any) bool {
	name := runtime.FuncForPC(reflect.ValueOf(handler).Pointer()).Name()
	for _, pkg := range readerPackages {
		if strings.Contains(name, "/core/application/service/"+pkg+".") {
			return true
		}
	}
	return false
}
