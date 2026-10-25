// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package jumble

import (
	"encoding/base32"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// TargetKind is what a mail address leads to (ADR-0092).
type TargetKind string

const (
	// TargetJumble is the workspace's jumble: whatever arrives becomes a jumble entry.
	TargetJumble TargetKind = "JUMBLE"
	// TargetCollection is a collection: a mail becomes an entry in it.
	TargetCollection TargetKind = "COLLECTION"
	// TargetItem is an entry: a mail becomes a comment on it.
	TargetItem TargetKind = "ITEM"
)

// Valid reports whether the kind is one the column allows.
func (k TargetKind) Valid() bool {
	switch k {
	case TargetJumble, TargetCollection, TargetItem:
		return true
	default:
		return false
	}
}

// MailTarget names one place. The jumble has no identifier - there is one per workspace.
type MailTarget struct {
	Kind TargetKind
	ID   shared.ID
}

// IsZero reports whether no place is named.
func (t MailTarget) IsZero() bool { return t.Kind == "" }

// NewMailTarget validates a place: the jumble without an identifier, the others with one.
func NewMailTarget(kind TargetKind, id shared.ID) (MailTarget, error) {
	switch {
	case !kind.Valid():
		return MailTarget{}, shared.ErrValidation.WithDetail("mail.target_unknown").
			WithFields(shared.FieldError{Path: "/target_kind", Code: "mail.target_unknown"})
	case kind == TargetJumble:
		return MailTarget{Kind: kind}, nil
	case id.IsZero():
		return MailTarget{}, shared.ErrValidation.WithDetail("mail.target_required").
			WithFields(shared.FieldError{Path: "/target_id", Code: "mail.target_required"})
	default:
		return MailTarget{Kind: kind, ID: id}, nil
	}
}

// SenderPolicy is who may write into a place by mail (ADR-0092 decision 5).
type SenderPolicy string

const (
	// SendersMembers lets people of the workspace act as themselves; everyone else waits in the
	// jumble. The default.
	SendersMembers SenderPolicy = "MEMBERS"
	// SendersAnyone lets an unknown sender's mail in as the account that opened the address.
	SendersAnyone SenderPolicy = "ANYONE"
)

// Valid reports whether the policy is one the column allows.
func (p SenderPolicy) Valid() bool { return p == SendersMembers || p == SendersAnyone }

// MailCodeBytes is the entropy of an address's code: 80 bits, sixteen base32 characters.
//
// Short enough to read aloud and to survive a mail client's line wrap, long enough that guessing one
// under the per-address limit and the route's rate limit is not a plan.
const MailCodeBytes = 10

// MailCodeLength is the code's length in characters.
const MailCodeLength = 16

// MaxMailLabelLength bounds the readable half of an address.
const MaxMailLabelLength = 24

// MailDeliveriesPerHour is how many deliveries one address takes in an hour (ADR-0092 decision 7).
const MailDeliveriesPerHour = 60

// mailCodeEncoding is base32 without padding, lowercase: mail addresses are case-insensitive in
// practice, and a code that changed under a client's lowercasing would be a different code.
var mailCodeEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// MailAddress is one place's address on the receiving domain.
//
// The code is the key and nothing else is: the label is decoration a reader can recognise, and a
// delivery is routed by the code alone, so renaming a place changes how its address reads without
// changing where it leads.
type MailAddress struct {
	ID       shared.ID
	TenantID shared.ID
	Target   MailTarget
	Code     string
	Policy   SenderPolicy
	// OpenedBy is the account that set the policy to ANYONE: an unknown sender's mail is written as
	// them, within their rights. Empty under MEMBERS.
	OpenedBy  shared.ID
	CreatedBy shared.ID
	CreatedAt time.Time
}

// NewMailAddress mints an address from freshly drawn randomness. The bytes come from the caller
// because the domain draws nothing itself (rule 4).
func NewMailAddress(
	id, tenantID shared.ID, target MailTarget, secret []byte, createdBy shared.ID, now time.Time,
) (MailAddress, error) {
	if id.IsZero() || tenantID.IsZero() || target.IsZero() {
		return MailAddress{}, shared.ErrInternal.WithDetail("mail.address_incomplete")
	}
	if len(secret) != MailCodeBytes {
		return MailAddress{}, shared.ErrInternal.WithDetail("mail.code_short")
	}
	return MailAddress{
		ID: id, TenantID: tenantID, Target: target,
		Code:   strings.ToLower(mailCodeEncoding.EncodeToString(secret)),
		Policy: SendersMembers, CreatedBy: createdBy, CreatedAt: now.UTC(),
	}, nil
}

// WithPolicy sets who may send. Opening to anyone records who did, because their rights are what
// such mail is written with.
func (a MailAddress) WithPolicy(policy SenderPolicy, by shared.ID) (MailAddress, error) {
	if !policy.Valid() {
		return MailAddress{}, shared.ErrValidation.WithDetail("mail.policy_unknown").
			WithFields(shared.FieldError{Path: "/sender_policy", Code: "mail.policy_unknown"})
	}
	a.Policy, a.OpenedBy = policy, ""
	if policy == SendersAnyone {
		a.OpenedBy = by
	}
	return a, nil
}

// Address is the address as a person copies it: `<label>.<code>@<domain>`.
func (a MailAddress) Address(label, domain string) string {
	return MailLabel(label) + "." + a.Code + "@" + strings.ToLower(domain)
}

// MailLabel makes a place's name readable in an address: letters and digits folded to ASCII, runs
// of anything else one hyphen, at most MaxMailLabelLength characters, and "mail" where nothing is
// left.
func MailLabel(name string) string {
	var out strings.Builder
	hyphen := false
	for _, r := range strings.ToLower(name) {
		if folded, ok := latinFold[r]; ok {
			out.WriteString(folded)
			hyphen = false
			continue
		}
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out.WriteRune(r)
			hyphen = false
		default:
			if out.Len() > 0 && !hyphen {
				out.WriteByte('-')
				hyphen = true
			}
		}
		if out.Len() >= MaxMailLabelLength {
			break
		}
	}
	label := strings.Trim(out.String(), "-")
	if len(label) > MaxMailLabelLength {
		label = strings.Trim(label[:MaxMailLabelLength], "-")
	}
	if label == "" {
		return "mail"
	}
	return label
}

// MailCodeOf reads the code out of a recipient at the receiving domain.
//
// The code is the local part's last dot-separated segment; anything before it is the label and is
// ignored. A recipient at another domain, or one whose last segment is not a code's shape, answers
// false - the same not-found every unknown address gets.
func MailCodeOf(recipient, domain string) (string, bool) {
	recipient = strings.ToLower(strings.TrimSpace(recipient))
	recipient = strings.TrimSuffix(strings.TrimPrefix(recipient, "<"), ">")
	local, host, found := strings.Cut(recipient, "@")
	if !found || domain == "" || host != strings.ToLower(domain) {
		return "", false
	}
	// A sub-address some services append (`label.code+tag`) is not part of the code.
	local, _, _ = strings.Cut(local, "+")
	code := local
	if index := strings.LastIndexByte(local, '.'); index >= 0 {
		code = local[index+1:]
	}
	if len(code) != MailCodeLength {
		return "", false
	}
	if _, err := mailCodeEncoding.DecodeString(strings.ToUpper(code)); err != nil {
		return "", false
	}
	return code, true
}

// latinFold spells the accented Latin letters names use most as plain ones. A table rather than
// Unicode normalisation: the domain takes no dependency for decoration, and a letter it does not
// know becomes the hyphen every other character becomes.
var latinFold = func() map[rune]string {
	folds := map[string]string{
		"a": "àáâãäåāăą", "c": "çćĉċč", "d": "ďđ", "e": "èéêëēĕėęě", "g": "ĝğġģ", "h": "ĥħ",
		"i": "ìíîïĩīĭįı", "j": "ĵ", "k": "ķ", "l": "ĺļľŀł", "n": "ñńņňŉ", "o": "òóôõöøōŏő",
		"r": "ŕŗř", "s": "śŝşšș", "t": "ţťŧț", "u": "ùúûüũūŭůűų", "w": "ŵ", "y": "ýÿŷ",
		"z": "źżž", "ss": "ß", "ae": "æ", "oe": "œ", "th": "þ",
	}
	table := map[rune]string{}
	for plain, accented := range folds {
		for _, r := range accented {
			table[r] = plain
		}
	}
	return table
}()
