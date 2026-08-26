// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package backupstorage holds the adapters behind core/port/backupstorage: the four protocols
// ADR-0019 opens with, and the conformance suite BK-1 holds every one of them to.
//
// Four implementations of the same six sentences is exactly the situation in which the sentences
// drift, so what they have in common lives here rather than four times over: what a key may be,
// and how a protocol's failure becomes one of the port's codes.
package backupstorage

import (
	"fmt"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/backupstorage"
)

// maxKeyLength is what a key may be. Not a protocol limit - several of these are lower - but a
// bound that keeps a key inside every one of them, so that an archive written to a directory can
// be copied to a bucket without being renamed.
const maxKeyLength = 900

// CheckKey refuses everything that could leave the target's namespace, or that one of these four
// protocols cannot express.
//
// Keys are minted by this system and never user text, so a key that walks is a defect rather than
// a request - and it is refused all the same, in one place, because four adapters checking this
// four ways is four chances to get it wrong. A backup target is somebody else's filesystem, and
// the consequence of a key escaping it is writing into somebody else's directory.
func CheckKey(key string) error {
	if key == "" || len(key) > maxKeyLength {
		return keyInvalid(key)
	}
	// A leading slash makes a key absolute on three of the four protocols; a backslash is a
	// separator on the fourth. A control character or a newline is a request smuggled into an
	// FTP or HTTP command line.
	if strings.HasPrefix(key, "/") || strings.ContainsAny(key, "\\\x00") {
		return keyInvalid(key)
	}
	for _, r := range key {
		if r < 0x20 || r == 0x7f {
			return keyInvalid(key)
		}
	}
	for _, segment := range strings.Split(key, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return keyInvalid(key)
		}
	}
	return nil
}

// CheckPrefix is the same question for a listing. An empty prefix is the whole namespace, which
// is what a restore after a total loss asks for, so it is allowed here and nowhere else.
func CheckPrefix(prefix string) error {
	if prefix == "" {
		return nil
	}
	return CheckKey(strings.TrimSuffix(prefix, "/"))
}

func keyInvalid(key string) error {
	return shared.ErrValidation.
		WithDetail(port.CodeKeyInvalid).
		WithCause(fmt.Errorf("the key %q is not a name this system writes", redactedLength(key)))
}

// redactedLength is what an error may say about a key. A key names a tenant and a moment, so the
// error says how long it was and nothing else (rule 10).
func redactedLength(key string) string { return fmt.Sprintf("<%d characters>", len(key)) }

// notFound is the one answer for a key that is not there.
func notFound(key string) error {
	return shared.ErrNotFound.
		WithDetail(port.CodeObjectNotFound).
		WithParams(map[string]string{"key": key})
}

// refused is a target that answered and said no - wrong credentials, no permission on the path.
// One code for both, because telling a caller which of the two it was tells them which half of
// their guess was right.
func refused(doing string, err error) error {
	return shared.ErrUnavailable.
		WithDetail(port.CodeTargetRefused).
		WithCause(fmt.Errorf("%s: %w", doing, err))
}

// failed is a target answering something this build cannot make sense of.
func failed(doing string, err error) error {
	return shared.ErrUnavailable.
		WithDetail(port.CodeTargetFailed).
		WithCause(fmt.Errorf("%s: %w", doing, err))
}
