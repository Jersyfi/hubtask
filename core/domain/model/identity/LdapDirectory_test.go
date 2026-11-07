// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var ldapNow = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

func ldapInput() LdapDirectoryInput {
	return LdapDirectoryInput{
		URL: "ldaps://dc.example.com", BindDN: "cn=reader,dc=example,dc=com",
		UserBase: "ou=people,dc=example,dc=com", RunAs: shared.ID("00000000-0000-0000-0000-0000000000aa"),
	}
}

func detailOf(err error) string {
	var coded *shared.Error
	if errors.As(err, &coded) {
		return coded.DetailCode
	}
	return ""
}

func TestAnLdapDirectoryNeverBindsInTheClear(t *testing.T) {
	cases := []struct {
		url, want string
		tls       bool
		address   string
	}{
		{"ldaps://dc.example.com", "", true, "dc.example.com:636"},
		{"ldap://dc.example.com:3389/", "", false, "dc.example.com:3389"},
		{"LDAPS://dc.example.com", "", true, "dc.example.com:636"},
		{"http://dc.example.com", "directory.ldap_url_invalid", false, ""},
		{"ldaps://user:pw@dc.example.com", "directory.ldap_url_invalid", false, ""},
		{"ldaps://dc.example.com/dc=example?cn", "directory.ldap_url_invalid", false, ""},
		{"ldaps://dc.example.com:99999", "directory.ldap_url_invalid", false, ""},
		{"ldaps://", "directory.ldap_url_invalid", false, ""},
	}
	for _, tc := range cases {
		in := ldapInput()
		in.URL = tc.url
		d, err := NewLdapDirectory("t", in, ldapNow)
		if got := detailOf(err); got != tc.want {
			t.Fatalf("%s: refusal %q, want %q", tc.url, got, tc.want)
		}
		if err == nil && (d.ImplicitTLS() != tc.tls || d.Address() != tc.address) {
			t.Fatalf("%s: tls %v address %s", tc.url, d.ImplicitTLS(), d.Address())
		}
	}
}

func TestAnLdapDirectoryTakesItsDefaultsAndRefusesAMalformedField(t *testing.T) {
	d, err := NewLdapDirectory("t", ldapInput(), ldapNow)
	if err != nil {
		t.Fatal(err)
	}
	if d.UserFilter != DefaultLdapUserFilter || d.IDAttribute != "entryUUID" || d.EveryMinutes != 60 ||
		!d.Enabled || d.NextRunAt == nil || !d.NextRunAt.Equal(ldapNow) || d.SyncsGroups() {
		t.Fatalf("defaults: %+v", d)
	}
	cases := []struct {
		name string
		edit func(*LdapDirectoryInput)
		want string
	}{
		{"no bind DN", func(in *LdapDirectoryInput) { in.BindDN = " " }, "directory.ldap_dn_required"},
		{"a base without a component", func(in *LdapDirectoryInput) { in.UserBase = "people" }, "directory.ldap_dn_invalid"},
		{"an unbalanced filter", func(in *LdapDirectoryInput) { in.UserFilter = "(&(objectClass=person)" }, "directory.ldap_filter_invalid"},
		{"two filters", func(in *LdapDirectoryInput) { in.UserFilter = "(a=b)(c=d)" }, "directory.ldap_filter_invalid"},
		{"a bare filter", func(in *LdapDirectoryInput) { in.GroupFilter = "objectClass=group" }, "directory.ldap_filter_invalid"},
		{"an attribute with a space", func(in *LdapDirectoryInput) { in.MailAttribute = "e mail" }, "directory.ldap_attribute_invalid"},
		{"no service account", func(in *LdapDirectoryInput) { in.RunAs = "" }, "directory.ldap_run_as_required"},
		{"too often", func(in *LdapDirectoryInput) { in.EveryMinutes = 5 }, "directory.ldap_interval_invalid"},
	}
	for _, tc := range cases {
		in := ldapInput()
		tc.edit(&in)
		if _, err := NewLdapDirectory("t", in, ldapNow); detailOf(err) != tc.want {
			t.Fatalf("%s: %v, want %s", tc.name, err, tc.want)
		}
	}
}

func TestAFailingLdapRunBacksOffAndAWorkingOneResets(t *testing.T) {
	d, _ := NewLdapDirectory("t", ldapInput(), ldapNow)
	d = d.Ran(ldapNow, LdapFailed, "directory.ldap_unreachable", LdapCounts{})
	if d.Failures != 1 || !d.NextRunAt.Equal(ldapNow.Add(2*time.Hour)) {
		t.Fatalf("first failure: %d, next %v", d.Failures, d.NextRunAt)
	}
	for range 10 {
		d = d.Ran(ldapNow, LdapRefused, "directory.ldap_answer_empty", LdapCounts{})
	}
	if !d.NextRunAt.Equal(ldapNow.Add(24 * time.Hour)) {
		t.Fatalf("the backoff is not bounded by a day: %v", d.NextRunAt)
	}
	d = d.Ran(ldapNow, LdapPartial, "", LdapCounts{})
	if d.Failures != 0 || !d.NextRunAt.Equal(ldapNow.Add(time.Hour)) {
		t.Fatalf("a run that worked did not reset: %d %v", d.Failures, d.NextRunAt)
	}
	d.Enabled = false
	if d = d.Ran(ldapNow, LdapOK, "", LdapCounts{}); d.NextRunAt != nil {
		t.Fatal("a directory switched off is scheduled")
	}
}

func heldPeople(n int) []LdapHeld {
	held := make([]LdapHeld, n)
	for i := range held {
		id := string(rune('a'+i%26)) + strings.Repeat("x", i/26)
		held[i] = LdapHeld{AccountID: shared.ID("acc-" + id), ExternalID: "ldap:" + id, Name: id, Status: AccountActive}
	}
	return held
}

func TestAnLdapPlanProvisionsLinksRenamesAndSwitches(t *testing.T) {
	in := LdapPlanInput{
		Answer: LdapAnswer{
			People: []LdapPerson{
				{DN: "uid=lena,ou=people", ID: "lena", Email: "lena@example.com", Name: "Lena"},
				{DN: "uid=old,ou=people", ID: "old", Email: "old@example.com", Name: "Old Name Changed"},
				{DN: "uid=back,ou=people", ID: "back", Email: "back@example.com", Name: "Back"},
				{DN: "uid=here,ou=people", ID: "here", Email: "Here@Example.com", Name: "Here"},
				{DN: "uid=scim,ou=people", ID: "scim", Email: "scim@example.com", Name: "Scim"},
				{DN: "uid=nomail,ou=people", ID: "nomail", Name: "No Mail"},
				{DN: "uid=twin1", ID: "twin1", Email: "twin@example.com"},
				{DN: "uid=twin2", ID: "twin2", Email: "twin@example.com"},
			},
			Groups: []LdapGroup{
				{ID: "g1", Name: "Engineering", MemberNames: []string{"UID=Lena, OU=People", "uid=old,ou=people", "uid=stranger"}},
				{ID: "g2", Name: "Kept"},
			},
		},
		Held: []LdapHeld{
			{AccountID: "acc-old", ExternalID: "ldap:old", Name: "Old Name", Status: AccountActive},
			{AccountID: "acc-back", ExternalID: "ldap:back", Name: "Back", Status: AccountDisabled},
			{AccountID: "acc-gone", ExternalID: "ldap:gone", Name: "Gone", Status: AccountInvited},
			{AccountID: "acc-limited", ExternalID: "ldap:limited", Name: "Limited", Status: AccountRestricted},
		},
		HeldGroups: []LdapHeldGroup{
			{GroupID: "grp-2", ExternalID: "ldap:g2", Name: "Kept"},
			{GroupID: "grp-3", ExternalID: "ldap:g3", Name: "Dissolved"},
		},
		ByAddress: map[string]LdapAddressMatch{
			"here@example.com": {AccountID: "acc-here", Kind: AccountUser, Status: AccountDisabled},
			"scim@example.com": {AccountID: "acc-scim", Kind: AccountUser, Status: AccountActive, ExternalID: "entra-7"},
		},
		Groups: true,
	}
	plan, err := PlanLdapSync(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Provision) != 1 || plan.Provision[0].ID != "lena" {
		t.Fatalf("provision: %+v", plan.Provision)
	}
	if len(plan.Link) != 1 || plan.Link[0].AccountID != "acc-here" || !plan.Link[0].SwitchOn || plan.Link[0].ExternalID != "ldap:here" {
		t.Fatalf("link: %+v", plan.Link)
	}
	if len(plan.Rename) != 1 || plan.Rename[0].Name != "Old Name Changed" {
		t.Fatalf("rename: %+v", plan.Rename)
	}
	if len(plan.SwitchOn) != 1 || plan.SwitchOn[0] != "acc-back" {
		t.Fatalf("switch on: %+v", plan.SwitchOn)
	}
	// The restricted person is not switched off: switching them on again would lift a
	// restriction nobody asked to lift.
	if len(plan.SwitchOff) != 1 || plan.SwitchOff[0] != "acc-gone" {
		t.Fatalf("switch off: %+v", plan.SwitchOff)
	}
	// Without an address, with an address another entry has, or held by SCIM: skipped.
	if plan.Skipped != 4 || plan.Seen != 8 {
		t.Fatalf("skipped %d seen %d", plan.Skipped, plan.Seen)
	}
	if len(plan.Groups) != 2 || plan.Groups[0].GroupID != "" || strings.Join(plan.Groups[0].Members, " ") != "ldap:lena ldap:old" ||
		plan.Groups[1].GroupID != "grp-2" {
		t.Fatalf("groups: %+v", plan.Groups)
	}
	if len(plan.Emptied) != 1 || plan.Emptied[0] != "grp-3" {
		t.Fatalf("emptied: %+v", plan.Emptied)
	}
}

func TestAnLdapPlanRefusesAnEmptyAnswer(t *testing.T) {
	_, err := PlanLdapSync(LdapPlanInput{Held: heldPeople(3)})
	if detailOf(err) != "directory.ldap_answer_empty" {
		t.Fatalf("an empty answer: %v", err)
	}
}

func TestAnLdapPlanRefusesSwitchingOffMoreThanTheGuardUnlessConfirmed(t *testing.T) {
	cases := []struct {
		held, gone int
		confirm    *int
		want       string
	}{
		{held: 20, gone: 10, want: ""},
		{held: 20, gone: 11, want: "directory.ldap_switch_off_too_many"},
		{held: 100, gone: 20, want: ""},
		{held: 100, gone: 21, want: "directory.ldap_switch_off_too_many"},
		{held: 100, gone: 21, confirm: new(21), want: ""},
		{held: 100, gone: 21, confirm: new(20), want: "directory.ldap_switch_off_too_many"},
	}
	for _, tc := range cases {
		held := heldPeople(tc.held)
		var people []LdapPerson
		for _, person := range held[tc.gone:] {
			people = append(people, LdapPerson{ID: strings.TrimPrefix(person.ExternalID, "ldap:"), Email: person.Name + "@example.com", Name: person.Name})
		}
		if len(people) == 0 {
			people = []LdapPerson{{ID: "someone", Email: "someone@example.com"}}
		}
		plan, err := PlanLdapSync(LdapPlanInput{Answer: LdapAnswer{People: people}, Held: held, Confirm: tc.confirm})
		if detailOf(err) != tc.want {
			t.Fatalf("held %d gone %d: %v, want %q", tc.held, tc.gone, err, tc.want)
		}
		if len(plan.SwitchOff) != tc.gone {
			t.Fatalf("held %d gone %d: the plan switches off %d", tc.held, tc.gone, len(plan.SwitchOff))
		}
	}
}

func TestAnLdapExternalIdentifierIsBounded(t *testing.T) {
	if LdapExternalID("") != "" || LdapExternalID(strings.Repeat("a", 251)) != "" || LdapExternalID("a\x01") != "" {
		t.Fatal("an identifier that cannot be stored was taken")
	}
	if LdapExternalID("8f1c") != "ldap:8f1c" {
		t.Fatal("an ordinary identifier was refused")
	}
}
