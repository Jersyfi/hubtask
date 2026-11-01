// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"math/bits"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// PublicFormsKey is the workspace's switch letting a collection take requests through a public
// form, without signing in (modules/service.md: public only where the workspace allows it).
const PublicFormsKey setting.Key = "servicedesk.public_forms"

// PublicFormsDefinition is its declaration: off by default; the installation and a plan may set
// and lock it, the workspace switches it inside that - the public page link's shape.
func PublicFormsDefinition() setting.Definition {
	return setting.Definition{
		Key: PublicFormsKey, Kind: setting.KindFlag,
		Levels:   []setting.Level{setting.LevelInstance, setting.LevelPlan, setting.LevelWorkspace},
		Lockable: true, May: setting.MayReplace, Default: false,
		Writers: map[setting.Level]string{
			setting.LevelInstance: "admin:instance", setting.LevelPlan: "admin:instance",
			setting.LevelWorkspace: "MANAGE_MEMBERS",
		},
	}
}

// PublicFormsOf reads the resolved switch; anything but true is off.
func PublicFormsOf(r setting.Resolution) bool {
	on, _ := r.Effective.(bool)
	return on
}

// PublicForm is a collection's public form: whoever holds its link raises a request of a type
// whose form opens PUBLIC, without an account. One per collection; a new link replaces the old,
// which stops answering. The token is never kept, only its hash.
//
// A submission is filed as the person who made the link, evaluated at every submission, on the
// visitor's behalf - the mail intake's way for a sender without an account.
type PublicForm struct {
	TenantID     shared.ID
	CollectionID shared.ID
	CreatedBy    shared.ID
	CreatedAt    time.Time
}

// NewPublicForm checks a link's identity.
func NewPublicForm(tenant, collection, by shared.ID, now time.Time) (PublicForm, error) {
	if tenant.IsZero() || collection.IsZero() || by.IsZero() {
		return PublicForm{}, shared.ErrInternal.WithDetail("public_forms.identity_incomplete")
	}
	return PublicForm{TenantID: tenant, CollectionID: collection, CreatedBy: by, CreatedAt: now}, nil
}

// Bounds of what a visitor names about themselves.
const (
	MaxRequesterNameLength    = 200
	MaxRequesterAddressLength = 320
)

// Requester is who a visitor says they are. Neither is checked against the workspace's accounts:
// a public form never tells whether an address has one (P-01), and an address typed by a stranger
// proves nothing, so it never signs anybody in or files as them.
type Requester struct {
	Name    string
	Address string
}

// NewRequester checks the words a From line is made of: no line break or angle bracket, which
// would let a visitor write a second line of their own, and an address of one '@' between words.
// Both may be empty: an anonymous request names nobody.
func NewRequester(name, address string) (Requester, error) {
	name, address = strings.TrimSpace(name), strings.ToLower(strings.TrimSpace(address))
	if len([]rune(name)) > MaxRequesterNameLength || strings.ContainsAny(name, "<>") || hasControl(name) {
		return Requester{}, submissionInvalid("/requester_name", "public_forms.requester_name_invalid")
	}
	if address == "" {
		if name != "" {
			return Requester{}, submissionInvalid("/requester_email", "public_forms.requester_address_missing")
		}
		return Requester{}, nil
	}
	local, host, found := strings.Cut(address, "@")
	if !found || local == "" || !strings.Contains(host, ".") || strings.ContainsAny(address, " <>,;\"") ||
		strings.Contains(host, "@") || len(address) > MaxRequesterAddressLength || hasControl(address) {
		return Requester{}, submissionInvalid("/requester_email", "public_forms.requester_address_invalid")
	}
	return Requester{Name: name, Address: address}, nil
}

// IsZero reports an anonymous request.
func (r Requester) IsZero() bool { return r.Address == "" }

// FromLine names the requester above the request, the mail intake's line (privacy.IntakePrefix),
// so an erasure by address and the requester's expiry find it the same way.
func (r Requester) FromLine() string {
	if r.Name != "" {
		return "From: " + r.Name + " <" + r.Address + ">"
	}
	return "From: " + r.Address
}

func hasControl(s string) bool {
	return strings.ContainsFunc(s, unicode.IsControl)
}

func submissionInvalid(path, code string) error {
	return shared.ErrValidation.WithDetail(code).WithFields(shared.FieldError{Path: path, Code: code})
}

// The proof of work a submission carries instead of a third-party CAPTCHA (NG-phone-home): the
// form hands out a challenge, the visitor's browser finds a number whose hash with it starts with
// PublicFormWorkBits zero bits - about a second of a phone's work, nothing for one person and a
// cost per request for a script. The challenge is signed, so the server keeps nothing until it is
// answered.
const (
	PublicFormWorkBits      = 18
	PublicFormChallengeLife = 10 * time.Minute
	// challengeNonceBytes and challengeTagBytes bound the parts of a challenge.
	challengeNonceBytes = 16
	challengeTagBytes   = 16
	maxSolutionLength   = 20
)

// PublicFormRequesterPerHour bounds the requests one requester's mail address files through the
// public forms in an hour, whatever client and network they come from: a script rotating
// addresses cannot flood one person's address with acknowledgements and requests in their name.
const PublicFormRequesterPerHour = 10

// ChallengeNonceBytes is the randomness the caller draws for a challenge (rule 4).
const ChallengeNonceBytes = challengeNonceBytes

// ChallengeMAC is the keyed hash a challenge is signed with; the key never leaves its adapter.
type ChallengeMAC func(message string) []byte

// Challenge is what a public form hands out with itself.
type Challenge struct {
	Value     string
	Bits      int
	ExpiresAt time.Time
}

// IssueChallenge signs a challenge for one form (scope names it), valid PublicFormChallengeLife.
func IssueChallenge(scope string, nonce []byte, now time.Time, mac ChallengeMAC) (Challenge, error) {
	if len(nonce) != challengeNonceBytes || scope == "" || mac == nil {
		return Challenge{}, shared.ErrInternal.WithDetail("public_forms.challenge_unmintable")
	}
	expires := now.Add(PublicFormChallengeLife).UTC().Truncate(time.Second)
	head := strconv.FormatInt(expires.Unix(), 10) + "." + base64.RawURLEncoding.EncodeToString(nonce)
	return Challenge{Value: head + "." + challengeTag(scope, head, mac), Bits: PublicFormWorkBits, ExpiresAt: expires}, nil
}

// VerifyWork checks an answered challenge: signed for this form, not expired, and solved. It
// answers the challenge's nonce, the key a spent challenge is remembered by.
func VerifyWork(scope, value, solution string, now time.Time, mac ChallengeMAC) (string, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 3 || mac == nil {
		return "", workInvalid()
	}
	head := parts[0] + "." + parts[1]
	if subtle.ConstantTimeCompare([]byte(parts[2]), []byte(challengeTag(scope, head, mac))) != 1 {
		return "", workInvalid()
	}
	expires, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return "", workInvalid()
	}
	if !now.Before(time.Unix(expires, 0)) {
		return "", shared.ErrValidation.WithDetail("public_forms.work_expired").
			WithFields(shared.FieldError{Path: "/challenge", Code: "public_forms.work_expired"})
	}
	if !Solves(value, solution, PublicFormWorkBits) {
		return "", workInvalid()
	}
	return parts[1], nil
}

// Solves reports whether solution answers the challenge: the SHA-256 of "value:solution" starts
// with at least bits zero bits. The solution is a decimal number.
func Solves(value, solution string, need int) bool {
	if solution == "" || len(solution) > maxSolutionLength || strings.Trim(solution, "0123456789") != "" {
		return false
	}
	sum := sha256.Sum256([]byte(value + ":" + solution))
	zeros := 0
	for _, b := range sum {
		if b == 0 {
			zeros += 8
			continue
		}
		zeros += bits.LeadingZeros8(b)
		break
	}
	return zeros >= need
}

func challengeTag(scope, head string, mac ChallengeMAC) string {
	tag := mac(scope + "|" + head)
	if len(tag) > challengeTagBytes {
		tag = tag[:challengeTagBytes]
	}
	return base64.RawURLEncoding.EncodeToString(tag)
}

func workInvalid() error {
	return shared.ErrValidation.WithDetail("public_forms.work_invalid").
		WithFields(shared.FieldError{Path: "/solution", Code: "public_forms.work_invalid"})
}
