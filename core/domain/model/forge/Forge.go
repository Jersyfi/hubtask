// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package forge is the code on an entry (UC-INT-13): a hub's hooks that a code forge - GitHub,
// GitLab, Forgejo or Gitea - posts its pushes and pull requests to, and the commits, branches and
// pull requests that name an entry's key, kept on the entry with their state.
//
// The forge is never called: it posts, and what it posts is read for the keys it names. A link is
// one per entry, hook, kind, repository and reference, and follows what the forge said last by the
// forge's own moment, so that a delivery repeated or arriving late never turns a merged pull
// request back into an open one.
package forge

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Kind is the forge a hook is for; its format and its way of proving the secret follow from it.
type Kind string

const (
	GitHub Kind = "GITHUB"
	GitLab Kind = "GITLAB"
	// Forgejo is Forgejo and Gitea alike: one format, two header spellings.
	Forgejo Kind = "FORGEJO"
)

var kinds = [...]Kind{GitHub, GitLab, Forgejo}

// Kinds is every forge, in the order a screen offers them.
func Kinds() []Kind { return kinds[:] }

// Valid reports whether the kind names a forge.
func (k Kind) Valid() bool {
	for _, known := range kinds {
		if k == known {
			return true
		}
	}
	return false
}

// LinkKind is what of the forge's an entry is linked to.
type LinkKind string

const (
	Commit      LinkKind = "COMMIT"
	Branch      LinkKind = "BRANCH"
	PullRequest LinkKind = "PULL_REQUEST"
)

// State is a link's state as its forge last told it: a commit is pushed; a branch is open until it
// is deleted, which closes it; a pull request (GitLab's merge request) is open, merged or closed.
type State string

const (
	Pushed State = "PUSHED"
	Open   State = "OPEN"
	Merged State = "MERGED"
	Closed State = "CLOSED"
)

// Bounds of what is kept.
const (
	MaxHookNameRunes      = 80
	MaxHooksPerHub        = 20
	MaxSignalsPerDelivery = 200
	MaxKeysPerSignal      = 10
	MaxTitleRunes         = 300
	MaxRepositoryRunes    = 200
	MaxRefRunes           = 255
	MaxAuthorRunes        = 100
	MaxURLBytes           = 2048
)

// Hook is one forge's door into a hub: its deliveries link the hub's entries their keys name. Its
// secret is never stored; it is derived from the installation's key and the hook's salt, so that
// drawing a new salt replaces it.
type Hook struct {
	ID       shared.ID
	TenantID shared.ID
	HubID    shared.ID
	Kind     Kind
	Name     string
	Enabled  bool
	// Salt is what the secret is derived from; not itself a secret without the installation's key.
	Salt string
	// RunAs is the person who last defined the hook: its links are written as them, and only while
	// they may still write in the hub.
	RunAs          shared.ID
	CreatedAt      time.Time
	UpdatedAt      time.Time
	LastDeliveryAt *time.Time
}

func invalid(path, code string, params ...string) error {
	err := shared.ErrValidation.WithDetail(code).WithFields(shared.FieldError{Path: path, Code: code})
	if len(params) == 2 {
		err = err.WithParams(map[string]string{params[0]: params[1]})
	}
	return err
}

// CheckHook checks a hook's kind and name as a person defines them.
func CheckHook(kind Kind, name string) (string, error) {
	if !kind.Valid() {
		return "", invalid("/kind", "forge.kind_unknown")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", invalid("/name", "forge.hook_name_required")
	}
	if utf8.RuneCountInString(name) > MaxHookNameRunes {
		return "", invalid("/name", "forge.hook_name_too_long", "maximum", strconv.Itoa(MaxHookNameRunes))
	}
	return name, nil
}

// Signal is one thing a delivery tells: a commit pushed, a branch pushed or deleted, a pull request
// opened, changed, merged or closed. Texts are where the keys are looked for - a commit's message,
// a branch's name, a pull request's title, branch and description - and are not kept.
type Signal struct {
	Kind       LinkKind
	Repository string
	Ref        string
	Title      string
	URL        string
	Author     string
	State      State
	At         *time.Time
	Texts      []string
}

// Link is an entry's commit, branch or pull request.
type Link struct {
	ID         shared.ID
	TenantID   shared.ID
	ItemID     shared.ID
	HubID      shared.ID
	HookID     shared.ID
	Kind       LinkKind
	Repository string
	Ref        string
	Title      string
	URL        string
	Author     string
	State      State
	HappenedAt time.Time
	UpdatedAt  time.Time
}

func clip(s string, runes int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= runes {
		return s
	}
	return strings.TrimSpace(string([]rune(s)[:runes]))
}

// firstLine is a commit message's subject: what a forge shows as its title.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return s[:i]
	}
	return s
}

// CheckSignal bounds what a delivery tells before it is kept: a known kind and state, a
// repository and a reference, an http or https address, the title's first line. A signal that
// cannot be kept is refused rather than kept in part, so that a forge's broken payload shows.
func CheckSignal(sig Signal) (Signal, error) {
	switch sig.Kind {
	case Commit:
		sig.State = Pushed
	case Branch:
		if sig.State != Closed {
			sig.State = Open
		}
	case PullRequest:
		if sig.State != Merged && sig.State != Closed {
			sig.State = Open
		}
	default:
		return Signal{}, invalid("/kind", "forge.delivery_malformed")
	}
	sig.Repository = clip(sig.Repository, MaxRepositoryRunes)
	sig.Ref = strings.TrimSpace(sig.Ref)
	if sig.Repository == "" || sig.Ref == "" || utf8.RuneCountInString(sig.Ref) > MaxRefRunes {
		return Signal{}, invalid("/ref", "forge.delivery_malformed")
	}
	sig.Title = clip(firstLine(sig.Title), MaxTitleRunes)
	if sig.Title == "" {
		sig.Title = sig.Ref
	}
	sig.Author = clip(sig.Author, MaxAuthorRunes)
	sig.URL = strings.TrimSpace(sig.URL)
	if sig.URL != "" {
		u, err := url.Parse(sig.URL)
		if err != nil || len(sig.URL) > MaxURLBytes || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
			sig.URL = ""
		}
	}
	return sig, nil
}

// keyPattern is an entry's key as it stands in a branch name or a message: "WEB-42" in
// "feature/WEB-42-login", "Fix WEB-42" or "[web-42]". Go's expressions have no look-around, so
// standing alone is checked on the indexes.
var keyPattern = regexp.MustCompile(`([A-Za-z][A-Za-z0-9]{1,19})-([0-9]{1,12})`)

// KeysIn answers the keys the texts name, each once and in the order they first appear, at most
// MaxKeysPerSignal. A key is a word of its own before its number ends - "WEB-42" in
// "feature/WEB-42-login", but not in "xWEB-42" or "WEB-421".
func KeysIn(texts ...string) []mail.Key {
	var out []mail.Key
	seen := map[mail.Key]bool{}
	for _, text := range texts {
		for _, at := range keyPattern.FindAllStringSubmatchIndex(text, -1) {
			before, _ := utf8.DecodeLastRuneInString(text[:at[0]])
			after, _ := utf8.DecodeRuneInString(text[at[1]:])
			if at[0] > 0 && (unicode.IsLetter(before) || unicode.IsDigit(before)) {
				continue
			}
			if at[1] < len(text) && unicode.IsDigit(after) {
				continue
			}
			n, err := strconv.ParseInt(text[at[4]:at[5]], 10, 64)
			if err != nil || n < 1 {
				continue
			}
			key := mail.Key{Handle: strings.ToLower(text[at[2]:at[3]]), Number: n}
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, key)
			if len(out) == MaxKeysPerSignal {
				return out
			}
		}
	}
	return out
}

// Apply answers the link as the signal leaves it and whether anything changed. The forge's own
// moment orders what it says: a signal older than the one the link follows is ignored, and a
// merged pull request stays merged, so that a delivery repeated or late never undoes a merge.
func Apply(existing Link, sig Signal, now time.Time) (Link, bool) {
	at := now
	if sig.At != nil {
		at = *sig.At
	}
	next := existing
	if existing.ID.IsZero() {
		next = Link{Kind: sig.Kind, Repository: sig.Repository, Ref: sig.Ref, HappenedAt: at}
	} else {
		if at.Before(existing.HappenedAt) || existing.State == Merged {
			return existing, false
		}
	}
	changed := existing.ID.IsZero() || next.Title != sig.Title || next.State != sig.State ||
		(sig.URL != "" && next.URL != sig.URL) || (sig.Author != "" && next.Author != sig.Author)
	if !changed {
		return existing, false
	}
	next.Title, next.State = sig.Title, sig.State
	if sig.URL != "" {
		next.URL = sig.URL
	}
	if sig.Author != "" {
		next.Author = sig.Author
	}
	next.HappenedAt, next.UpdatedAt = at, now
	return next, true
}
