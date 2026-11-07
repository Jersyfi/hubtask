// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// An LDAP directory keeps a workspace's people and groups the way a SCIM directory does (UC-ID-31,
// identity.md §27), except that Hubtask reads it on a schedule instead of being written to. What a
// run may change is SCIM's: invite, rename, switch off and on, keep groups - never erase, never an
// owner. What is the directory's own is held here: a configuration that cannot bind in the clear,
// and a plan that refuses rather than switches off the workspace when the directory answers wrong.

// LdapExternalPrefix marks the people and groups an LDAP directory holds: their external identifier
// is the prefix and the directory's own identifier. A person SCIM holds has none, so the two never
// take each other's.
const LdapExternalPrefix = "ldap:"

// The defaults and bounds of an LDAP directory's configuration.
const (
	DefaultLdapUserFilter      = "(objectClass=person)"
	DefaultLdapGroupFilter     = "(objectClass=groupOfNames)"
	DefaultLdapIDAttribute     = "entryUUID"
	DefaultLdapMailAttribute   = "mail"
	DefaultLdapNameAttribute   = "displayName"
	DefaultLdapGroupNameAttr   = "cn"
	DefaultLdapMemberAttribute = "member"
	DefaultLdapEveryMinutes    = 60
	MinLdapEveryMinutes        = 15
	MaxLdapEveryMinutes        = 1440
	MaxLdapURL                 = 500
	MaxLdapDN                  = 1000
	MaxLdapFilter              = 2000
	MaxLdapAttribute           = 100
	MaxLdapPassword            = 1000
	// MaxLdapEntries bounds one answer: a directory that answers more is refused whole rather
	// than synchronised in part (directory.ldap_answer_too_large).
	MaxLdapEntries = 50000
	// LdapSwitchOffFloor and LdapSwitchOffShare are the guard: a run switches off at most the
	// larger of ten people and a fifth of those the directory holds unless the count is confirmed.
	LdapSwitchOffFloor = 10
	LdapSwitchOffShare = 5
	// maxLdapBackoff bounds how far failed runs push the next one out.
	maxLdapBackoff = 24 * time.Hour
)

// LdapOutcome is how a run ended.
type LdapOutcome string

const (
	LdapOK      LdapOutcome = "OK"
	LdapPartial LdapOutcome = "PARTIAL"
	LdapRefused LdapOutcome = "REFUSED"
	LdapFailed  LdapOutcome = "FAILED"
)

// LdapCounts is what a run did, or a dry run would do.
type LdapCounts struct {
	Seen, Provisioned, Linked, Renamed, SwitchedOff, SwitchedOn, Kept, Skipped, Failed int
	GroupsCreated, GroupsChanged                                                       int
}

// LdapDirectory is a workspace's LDAP directory. The bind password is not a member: it is sealed
// beside the row and opened only for a run.
type LdapDirectory struct {
	TenantID           shared.ID
	URL                string
	BindDN             string
	UserBase           string
	UserFilter         string
	GroupBase          string
	GroupFilter        string
	IDAttribute        string
	MailAttribute      string
	NameAttribute      string
	GroupNameAttribute string
	MemberAttribute    string
	RunAs              shared.ID
	EveryMinutes       int
	Enabled            bool
	NextRunAt          *time.Time
	LastRunAt          *time.Time
	LastOutcome        LdapOutcome
	LastError          string
	LastCounts         *LdapCounts
	Failures           int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// LdapDirectoryInput is a configuration as an administrator sends it; empty takes the default.
type LdapDirectoryInput struct {
	URL, BindDN, UserBase, UserFilter, GroupBase, GroupFilter                      string
	IDAttribute, MailAttribute, NameAttribute, GroupNameAttribute, MemberAttribute string
	RunAs                                                                          shared.ID
	EveryMinutes                                                                   int
	Enabled                                                                        *bool
}

// NewLdapDirectory checks a configuration whole and answers the directory it describes, due now
// when it runs on its own.
func NewLdapDirectory(tenantID shared.ID, in LdapDirectoryInput, now time.Time) (LdapDirectory, error) {
	address, err := ldapURL(in.URL)
	if err != nil {
		return LdapDirectory{}, err
	}
	d := LdapDirectory{TenantID: tenantID, URL: address, RunAs: in.RunAs, Enabled: true, CreatedAt: now, UpdatedAt: now}
	if in.Enabled != nil {
		d.Enabled = *in.Enabled
	}
	if d.BindDN, err = ldapDN(in.BindDN, "/bind_dn", true); err != nil {
		return LdapDirectory{}, err
	}
	if d.UserBase, err = ldapDN(in.UserBase, "/user_base", true); err != nil {
		return LdapDirectory{}, err
	}
	if d.GroupBase, err = ldapDN(in.GroupBase, "/group_base", false); err != nil {
		return LdapDirectory{}, err
	}
	if d.UserFilter, err = ldapFilter(in.UserFilter, DefaultLdapUserFilter, "/user_filter"); err != nil {
		return LdapDirectory{}, err
	}
	if d.GroupFilter, err = ldapFilter(in.GroupFilter, DefaultLdapGroupFilter, "/group_filter"); err != nil {
		return LdapDirectory{}, err
	}
	for _, attr := range []struct {
		into          *string
		raw, fallback string
		path          string
	}{
		{&d.IDAttribute, in.IDAttribute, DefaultLdapIDAttribute, "/id_attribute"},
		{&d.MailAttribute, in.MailAttribute, DefaultLdapMailAttribute, "/mail_attribute"},
		{&d.NameAttribute, in.NameAttribute, DefaultLdapNameAttribute, "/name_attribute"},
		{&d.GroupNameAttribute, in.GroupNameAttribute, DefaultLdapGroupNameAttr, "/group_name_attribute"},
		{&d.MemberAttribute, in.MemberAttribute, DefaultLdapMemberAttribute, "/member_attribute"},
	} {
		if *attr.into, err = ldapAttribute(attr.raw, attr.fallback, attr.path); err != nil {
			return LdapDirectory{}, err
		}
	}
	if in.RunAs.IsZero() {
		return LdapDirectory{}, ldapInvalid("directory.ldap_run_as_required", "/run_as")
	}
	d.EveryMinutes = in.EveryMinutes
	if d.EveryMinutes == 0 {
		d.EveryMinutes = DefaultLdapEveryMinutes
	}
	if d.EveryMinutes < MinLdapEveryMinutes || d.EveryMinutes > MaxLdapEveryMinutes {
		return LdapDirectory{}, ldapInvalid("directory.ldap_interval_invalid", "/every_minutes")
	}
	if d.Enabled {
		due := now
		d.NextRunAt = &due
	}
	return d, nil
}

// Reconfigured is the stored directory replaced by a new configuration: its creation and its last
// run's record stay, the streak of failures starts again.
func (d LdapDirectory) Reconfigured(stored LdapDirectory) LdapDirectory {
	d.CreatedAt = stored.CreatedAt
	d.LastRunAt, d.LastOutcome, d.LastError, d.LastCounts = stored.LastRunAt, stored.LastOutcome, stored.LastError, stored.LastCounts
	return d
}

// ImplicitTLS reports whether the address is ldaps; ldap is always upgraded by StartTLS.
func (d LdapDirectory) ImplicitTLS() bool { return strings.HasPrefix(d.URL, "ldaps://") }

// Host is the directory's host name, without the port.
func (d LdapDirectory) Host() string {
	parsed, err := url.Parse(d.URL)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

// Address is host and port, the port the scheme's when none is written.
func (d LdapDirectory) Address() string {
	parsed, err := url.Parse(d.URL)
	if err != nil {
		return ""
	}
	port := parsed.Port()
	if port == "" {
		port = "389"
		if d.ImplicitTLS() {
			port = "636"
		}
	}
	return net.JoinHostPort(parsed.Hostname(), port)
}

// SyncsGroups reports whether groups are read at all.
func (d LdapDirectory) SyncsGroups() bool { return d.GroupBase != "" }

// Due reports whether a scheduled run is owed.
func (d LdapDirectory) Due(now time.Time) bool {
	return d.Enabled && d.NextRunAt != nil && !now.Before(*d.NextRunAt)
}

// Ran records a run. A refused or failed run counts towards the streak and pushes the next one out
// - twice the interval, four times, up to a day - so a directory that is down is asked again, but
// not every few minutes; one that works again resets it.
func (d LdapDirectory) Ran(now time.Time, outcome LdapOutcome, code string, counts LdapCounts) LdapDirectory {
	at := now
	d.LastRunAt, d.LastOutcome, d.LastError = &at, outcome, code
	d.LastCounts = &counts
	if outcome == LdapRefused || outcome == LdapFailed {
		d.Failures++
	} else {
		d.Failures = 0
	}
	if !d.Enabled {
		d.NextRunAt = nil
		return d
	}
	wait := time.Duration(d.EveryMinutes) * time.Minute
	for i := 0; i < d.Failures && wait < maxLdapBackoff; i++ {
		wait *= 2
	}
	wait = min(wait, maxLdapBackoff)
	next := now.Add(wait)
	d.NextRunAt = &next
	return d
}

func ldapInvalid(code, path string) error {
	return shared.ErrValidation.WithDetail(code).WithFields(shared.FieldError{Path: path, Code: code})
}

// ldapURL holds the address to ldaps or ldap - the latter always upgraded by StartTLS, so no form
// of it binds in the clear - with a host, an optional port and nothing else.
func ldapURL(raw string) (string, error) {
	address := strings.TrimSpace(raw)
	parsed, err := url.Parse(address)
	if err != nil || len(address) > MaxLdapURL || (parsed.Scheme != "ldaps" && parsed.Scheme != "ldap") ||
		parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Path != "" && parsed.Path != "/") {
		return "", ldapInvalid("directory.ldap_url_invalid", "/url")
	}
	if port := parsed.Port(); port != "" {
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			return "", ldapInvalid("directory.ldap_url_invalid", "/url")
		}
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

func ldapDN(raw, path string, required bool) (string, error) {
	dn := strings.TrimSpace(raw)
	if dn == "" {
		if required {
			return "", ldapInvalid("directory.ldap_dn_required", path)
		}
		return "", nil
	}
	if utf8.RuneCountInString(dn) > MaxLdapDN || !strings.Contains(dn, "=") || strings.ContainsFunc(dn, unicode.IsControl) {
		return "", ldapInvalid("directory.ldap_dn_invalid", path)
	}
	return dn, nil
}

// ldapFilter holds a filter to its outer shape: parenthesised and balanced, no control character.
// The adapter compiles it at the probe; nothing of a request is ever placed inside one.
func ldapFilter(raw, fallback, path string) (string, error) {
	filter := strings.TrimSpace(raw)
	if filter == "" {
		return fallback, nil
	}
	depth := 0
	for i, r := range filter {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 || (depth == 0 && i != len(filter)-1) {
				return "", ldapInvalid("directory.ldap_filter_invalid", path)
			}
		}
	}
	if depth != 0 || filter[0] != '(' || utf8.RuneCountInString(filter) > MaxLdapFilter ||
		strings.ContainsFunc(filter, unicode.IsControl) {
		return "", ldapInvalid("directory.ldap_filter_invalid", path)
	}
	return filter, nil
}

func ldapAttribute(raw, fallback, path string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return fallback, nil
	}
	if len(name) > MaxLdapAttribute {
		return "", ldapInvalid("directory.ldap_attribute_invalid", path)
	}
	for i, r := range name {
		letter := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		if !letter && (i == 0 || (r < '0' || r > '9') && r != '-') {
			return "", ldapInvalid("directory.ldap_attribute_invalid", path)
		}
	}
	return name, nil
}

// LdapPerson is a person as the directory answers them. ID is the identifier attribute's value -
// hex where it is binary, as objectGUID is - and Email the address folded for lookup.
type LdapPerson struct {
	DN, ID, Email, Name string
}

// LdapGroup is a group as the directory answers it, its members by their entries' names.
type LdapGroup struct {
	ID, Name    string
	MemberNames []string
}

// LdapAnswer is what one read of the directory answered.
type LdapAnswer struct {
	People []LdapPerson
	Groups []LdapGroup
}

// LdapHeld is a person the directory holds, as the workspace has them.
type LdapHeld struct {
	AccountID  shared.ID
	ExternalID string
	Name       string
	Status     AccountStatus
}

// LdapHeldGroup is a group the directory holds.
type LdapHeldGroup struct {
	GroupID    shared.ID
	ExternalID string
	Name       string
}

// LdapAddressMatch is an account the directory does not hold yet, found under an address it names.
type LdapAddressMatch struct {
	AccountID  shared.ID
	Kind       AccountKind
	Status     AccountStatus
	Name       string
	ExternalID string
}

// LdapPlanInput is everything a plan is made from.
type LdapPlanInput struct {
	Answer     LdapAnswer
	Held       []LdapHeld
	HeldGroups []LdapHeldGroup
	// ByAddress are the accounts under the addresses of people the directory does not hold yet.
	ByAddress map[string]LdapAddressMatch
	Groups    bool
	// Confirm is the switch-off count given back to let a guarded run through; nil confirms none.
	Confirm *int
}

// LdapLink is an account already here that the directory takes into its keeping.
type LdapLink struct {
	AccountID  shared.ID
	ExternalID string
	Name       string
	SwitchOn   bool
}

// LdapRename is a person whose name the directory changed.
type LdapRename struct {
	AccountID shared.ID
	Name      string
}

// LdapGroupPlan is one group as it is to be: GroupID zero creates it. Members are external
// identifiers, since a person provisioned in the same run has no account identifier yet.
type LdapGroupPlan struct {
	GroupID    shared.ID
	ExternalID string
	Name       string
	Members    []string
}

// LdapPlan is what a run changes.
type LdapPlan struct {
	Provision []LdapPerson
	Link      []LdapLink
	Rename    []LdapRename
	SwitchOff []shared.ID
	SwitchOn  []shared.ID
	Groups    []LdapGroupPlan
	// Emptied are the groups the directory holds that it no longer answers: their members are
	// removed, the group and its roles stay for an administrator to decide.
	Emptied        []shared.ID
	Seen, Skipped  int
	SwitchOffLimit int
}

// LdapExternalID is the external identifier of a directory's entry, or empty when it cannot be one.
func LdapExternalID(id string) string {
	external, err := ExternalID(LdapExternalPrefix + id)
	if err != nil || id == "" || external != LdapExternalPrefix+id {
		return ""
	}
	return external
}

// SwitchOffLimit is how many of those the directory holds a run may switch off unconfirmed.
func SwitchOffLimit(held int) int {
	return max(LdapSwitchOffFloor, held/LdapSwitchOffShare)
}

// PlanLdapSync decides what a run changes. A directory that answers nobody is refused - a wrong
// base or filter answers nobody, and would otherwise switch off everybody - and so is a run that
// switches off more than SwitchOffLimit unless the count is confirmed. The plan is answered with
// the refusal, so that a dry run shows what was refused.
func PlanLdapSync(in LdapPlanInput) (LdapPlan, error) {
	plan := LdapPlan{Seen: len(in.Answer.People), SwitchOffLimit: SwitchOffLimit(len(in.Held))}
	if len(in.Answer.People) == 0 {
		return plan, shared.ErrConflict.WithDetail("directory.ldap_answer_empty")
	}
	held := make(map[string]LdapHeld, len(in.Held))
	for _, person := range in.Held {
		held[person.ExternalID] = person
	}
	addresses := map[string]int{}
	for _, person := range in.Answer.People {
		if person.Email != "" {
			addresses[strings.ToLower(person.Email)]++
		}
	}
	present := map[string]bool{}
	// kept maps an entry's name in the directory to the external identifier of the person it is
	// in the workspace after this run, for the groups' members.
	kept := map[string]string{}
	for _, person := range in.Answer.People {
		external := LdapExternalID(person.ID)
		if external == "" || present[external] {
			plan.Skipped++
			continue
		}
		present[external] = true
		if current, ok := held[external]; ok {
			kept[normalDN(person.DN)] = external
			if name := strings.TrimSpace(person.Name); name != "" && name != current.Name {
				plan.Rename = append(plan.Rename, LdapRename{AccountID: current.AccountID, Name: name})
			}
			if current.Status == AccountDisabled {
				plan.SwitchOn = append(plan.SwitchOn, current.AccountID)
			}
			continue
		}
		address := strings.ToLower(person.Email)
		if address == "" || !strings.Contains(address, "@") || addresses[address] > 1 {
			plan.Skipped++
			continue
		}
		if match, ok := in.ByAddress[address]; ok {
			if match.Kind != AccountUser || match.ExternalID != "" || match.Status == AccountAnonymized {
				plan.Skipped++
				continue
			}
			plan.Link = append(plan.Link, LdapLink{
				AccountID: match.AccountID, ExternalID: external, Name: strings.TrimSpace(person.Name),
				SwitchOn: match.Status == AccountDisabled,
			})
			kept[normalDN(person.DN)] = external
			continue
		}
		person.Email = address
		plan.Provision = append(plan.Provision, person)
		kept[normalDN(person.DN)] = external
	}
	for _, person := range in.Held {
		if present[person.ExternalID] {
			continue
		}
		if person.Status == AccountActive || person.Status == AccountInvited {
			plan.SwitchOff = append(plan.SwitchOff, person.AccountID)
		}
	}
	if in.Groups {
		planGroups(&plan, in, kept)
	}
	if len(plan.SwitchOff) > plan.SwitchOffLimit && (in.Confirm == nil || *in.Confirm != len(plan.SwitchOff)) {
		return plan, shared.ErrConflict.WithDetail("directory.ldap_switch_off_too_many").
			WithParams(map[string]string{
				"count": strconv.Itoa(len(plan.SwitchOff)), "limit": strconv.Itoa(plan.SwitchOffLimit),
			})
	}
	return plan, nil
}

func planGroups(plan *LdapPlan, in LdapPlanInput, kept map[string]string) {
	heldGroups := make(map[string]LdapHeldGroup, len(in.HeldGroups))
	for _, group := range in.HeldGroups {
		heldGroups[group.ExternalID] = group
	}
	answered := map[string]bool{}
	for _, group := range in.Answer.Groups {
		external := LdapExternalID(group.ID)
		name := strings.TrimSpace(group.Name)
		if external == "" || name == "" || answered[external] {
			continue
		}
		answered[external] = true
		members := []string{}
		seen := map[string]bool{}
		for _, dn := range group.MemberNames {
			if member, ok := kept[normalDN(dn)]; ok && !seen[member] {
				seen[member] = true
				members = append(members, member)
			}
		}
		entry := LdapGroupPlan{ExternalID: external, Name: name, Members: members}
		if current, ok := heldGroups[external]; ok {
			entry.GroupID = current.GroupID
		}
		plan.Groups = append(plan.Groups, entry)
	}
	for _, group := range in.HeldGroups {
		if !answered[group.ExternalID] {
			plan.Emptied = append(plan.Emptied, group.GroupID)
		}
	}
}

// normalDN compares two names of one entry as a directory writes them: case and the spaces around
// the separators do not count.
func normalDN(dn string) string {
	parts := strings.Split(dn, ",")
	for i, part := range parts {
		key, value, _ := strings.Cut(part, "=")
		parts[i] = strings.ToLower(strings.TrimSpace(key)) + "=" + strings.ToLower(strings.TrimSpace(value))
	}
	return strings.Join(parts, ",")
}
