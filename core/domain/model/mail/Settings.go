// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package mail

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Level is who may write into the workspace by mail (ADR-0094 decision 4).
type Level string

const (
	// LevelMembers admits the workspace's own active accounts, which then write as themselves.
	LevelMembers Level = "MEMBERS"
	// LevelAllowed admits the members and the addresses and domains on the allow list.
	LevelAllowed Level = "MEMBERS_AND_ALLOWED"
	// LevelAnyone admits every sender the block list does not name.
	LevelAnyone Level = "ANYONE"
)

// Limits on what the settings hold: enough for a workspace, small enough to read on one page.
const (
	MaxListEntries  = 200
	MaxSenderRoutes = 100
)

// Senders is the level with its two lists. An entry is an address ("buchhaltung@steuerbuero.de")
// or a domain ("@lieferant.de"), which also covers its subdomains.
type Senders struct {
	Level   Level
	Allowed []string
	Blocked []string
}

// Blocks reports whether the block list names the sender. It applies before every level.
func (s Senders) Blocks(sender string) bool { return matchesAny(s.Blocked, sender) }

// Admits reports whether the sender may write at this level; member is whether the address is an
// active account of the workspace.
func (s Senders) Admits(sender string, member bool) bool {
	if s.Blocks(sender) {
		return false
	}
	switch s.Level {
	case LevelAnyone:
		return true
	case LevelAllowed:
		return member || matchesAny(s.Allowed, sender)
	default:
		return member
	}
}

// SenderRoute files a sender's mail into a collection when the message names no place itself.
type SenderRoute struct {
	Match        string
	CollectionID shared.ID
}

// RouteBySender is the collection the first matching rule names: an exact address before a domain,
// the longer domain before the shorter one.
func RouteBySender(routes []SenderRoute, sender string) (shared.ID, bool) {
	best, bestLength := shared.ID(""), -1
	for _, route := range routes {
		if !matches(route.Match, sender) {
			continue
		}
		length := len(route.Match)
		if !strings.HasPrefix(route.Match, "@") {
			length += 1000
		}
		if length > bestLength {
			best, bestLength = route.CollectionID, length
		}
	}
	return best, bestLength >= 0
}

// Settings is a workspace's mail intake (ADR-0094 decision 8).
type Settings struct {
	Enabled bool
	// Name is the workspace's name on the operator's receiving domain, "familie-meier"; unique on
	// the installation. Empty until the intake is first set up there.
	Name string
	// DefaultCollectionID is where a message naming no place goes; empty for the jumble.
	DefaultCollectionID shared.ID
	Senders             Senders
	SenderRoutes        []SenderRoute
	// OpenedBy is the administrator who last saved the settings: a sender who is not a member
	// writes through this account, with a From line above the text.
	OpenedBy shared.ID
}

// DefaultSettings is a workspace that has never set its intake up: off, members only.
func DefaultSettings() Settings { return Settings{Senders: Senders{Level: LevelMembers}} }

var workspaceName = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,38}[a-z0-9])?$`)

// Normalised checks the settings and brings the lists to their stored form: lower case, a bare
// domain written with its "@", no duplicates, in order.
func (s Settings) Normalised() (Settings, error) {
	var fields []shared.FieldError
	if s.Senders.Level == "" {
		s.Senders.Level = LevelMembers
	}
	switch s.Senders.Level {
	case LevelMembers, LevelAllowed, LevelAnyone:
	default:
		fields = append(fields, shared.FieldError{Path: "/senders/level", Code: "mail.level_unknown"})
	}
	s.Name = strings.ToLower(strings.TrimSpace(s.Name))
	if s.Name != "" && !workspaceName.MatchString(s.Name) {
		fields = append(fields, shared.FieldError{Path: "/name", Code: "mail.name_invalid"})
	}
	var bad []shared.FieldError
	s.Senders.Allowed, bad = normaliseList(s.Senders.Allowed, "/senders/allowed")
	fields = append(fields, bad...)
	s.Senders.Blocked, bad = normaliseList(s.Senders.Blocked, "/senders/blocked")
	fields = append(fields, bad...)
	if len(s.SenderRoutes) > MaxSenderRoutes {
		fields = append(fields, shared.FieldError{Path: "/sender_routes", Code: "mail.too_many_routes"})
	}
	routes := make([]SenderRoute, 0, len(s.SenderRoutes))
	for index, route := range s.SenderRoutes {
		match, ok := NormaliseSender(route.Match)
		path := "/sender_routes/" + strconv.Itoa(index)
		if !ok {
			fields = append(fields, shared.FieldError{Path: path + "/match", Code: "mail.sender_invalid"})
			continue
		}
		if route.CollectionID.IsZero() {
			fields = append(fields, shared.FieldError{Path: path + "/collection_id", Code: "mail.route_without_place"})
			continue
		}
		routes = append(routes, SenderRoute{Match: match, CollectionID: route.CollectionID})
	}
	s.SenderRoutes = routes
	if len(fields) > 0 {
		return Settings{}, shared.ErrValidation.WithDetail("mail.settings_invalid").WithFields(fields...)
	}
	return s, nil
}

// NormaliseSender is a list entry in its stored form: "a@b.de", or "@b.de" for a whole domain,
// which may also be written bare ("b.de").
func NormaliseSender(entry string) (string, bool) {
	entry = strings.ToLower(strings.TrimSpace(entry))
	if entry == "" || strings.ContainsAny(entry, " <>,;") {
		return "", false
	}
	local, domain, found := strings.Cut(entry, "@")
	if !found {
		domain, local = entry, ""
	}
	if strings.Contains(domain, "@") || !strings.Contains(domain, ".") ||
		strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return "", false
	}
	return local + "@" + domain, true
}

func normaliseList(entries []string, path string) ([]string, []shared.FieldError) {
	var fields []shared.FieldError
	if len(entries) > MaxListEntries {
		fields = append(fields, shared.FieldError{Path: path, Code: "mail.list_too_long"})
	}
	out := make([]string, 0, len(entries))
	for index, entry := range entries {
		normal, ok := NormaliseSender(entry)
		if !ok {
			fields = append(fields, shared.FieldError{Path: path + "/" + strconv.Itoa(index), Code: "mail.sender_invalid"})
			continue
		}
		if !slices.Contains(out, normal) {
			out = append(out, normal)
		}
	}
	return out, fields
}

func matchesAny(entries []string, sender string) bool {
	for _, entry := range entries {
		if matches(entry, sender) {
			return true
		}
	}
	return false
}

// matches is one list entry against a sender: the same address, or an address on the entry's
// domain or below it ("@lieferant.de" covers "a@eu.lieferant.de").
func matches(entry, sender string) bool {
	local, domain, ok := split(sender)
	if !ok {
		return false
	}
	if bare, isDomain := strings.CutPrefix(entry, "@"); isDomain {
		return domain == bare || strings.HasSuffix(domain, "."+bare)
	}
	return entry == local+"@"+domain
}
