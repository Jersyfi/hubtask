// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package media

import (
	"net/url"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// DocumentServer is where a workspace's office files are shown and edited (F-3, E-14). Hubtask
// edits no file itself: a document server does, through the WOPI host, and saving there is a new
// version here. The kinds name the servers because this is an integration target, like a file
// store - nowhere else does a product name appear.
type DocumentServer string

const (
	// DocumentServerNone is the default: the viewer shows office files only as a card.
	DocumentServerNone DocumentServer = "NONE"
	// DocumentServerCollabora is Collabora Online.
	DocumentServerCollabora DocumentServer = "COLLABORA_ONLINE"
	// DocumentServerOnlyOffice is ONLYOFFICE Docs.
	DocumentServerOnlyOffice DocumentServer = "ONLYOFFICE"
	// DocumentServerEuroOffice is ONLYOFFICE's fork Euro-Office, attached only through WOPI.
	DocumentServerEuroOffice DocumentServer = "EURO_OFFICE"
	// DocumentServerMicrosoft is the largest vendor's browser office, through its partner programme;
	// optional, never required.
	DocumentServerMicrosoft DocumentServer = "MICROSOFT_365"
)

// DocumentServerChoice is the administration's entry: which server, and its address.
type DocumentServerChoice struct {
	Kind DocumentServer
	// URL is the server's own origin; its WOPI discovery is read from there.
	URL string
}

// NewDocumentServerChoice validates an administrator's choice. NONE takes no address; every other
// kind takes an absolute https address without credentials, query or fragment - the WOPI host
// sends it an access token, so plain http would put that token on the wire.
func NewDocumentServerChoice(kind DocumentServer, address string) (DocumentServerChoice, error) {
	switch kind {
	case DocumentServerNone:
		return DocumentServerChoice{Kind: DocumentServerNone}, nil
	case DocumentServerCollabora, DocumentServerOnlyOffice, DocumentServerEuroOffice, DocumentServerMicrosoft:
	default:
		return DocumentServerChoice{}, documentServerInvalid("/kind", "media.document_server_kind")
	}
	parsed, err := url.Parse(strings.TrimSpace(address))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return DocumentServerChoice{}, documentServerInvalid("/url", "media.document_server_url")
	}
	return DocumentServerChoice{Kind: kind, URL: strings.TrimSuffix(parsed.String(), "/")}, nil
}

// IsOn answers whether office files go to a document server at all.
func (c DocumentServerChoice) IsOn() bool { return c.Kind != "" && c.Kind != DocumentServerNone }

func documentServerInvalid(path, code string) error {
	return shared.ErrValidation.WithDetail(code).WithFields(shared.FieldError{Path: path, Code: code})
}

// officeEndings are the files a document server opens: OOXML, ODF and the legacy binaries.
var officeEndings = map[string]bool{
	"docx": true, "doc": true, "odt": true, "rtf": true,
	"xlsx": true, "xls": true, "ods": true, "csv": false,
	"pptx": true, "ppt": true, "odp": true, "odg": true,
}

// IsOfficeFile answers whether a file is one the document server shows and edits. The name's
// ending decides beside the type, because an OOXML file sniffs as a ZIP unless its claim sharpened it.
func IsOfficeFile(contentType, fileName string) bool {
	if zipFormats[essence(contentType)] || essence(contentType) == "application/msword" {
		return true
	}
	dot := strings.LastIndexByte(fileName, '.')
	return dot >= 0 && officeEndings[strings.ToLower(fileName[dot+1:])]
}

// LockDuration is how long a WOPI lock holds without a refresh (the protocol's thirty minutes).
const LockDuration = 30 * time.Minute

// MaxLockLength is the longest lock identifier a document server may set; the protocol allows
// 1024 characters.
const MaxLockLength = 1024

// Lock is a document server's hold on a file while somebody edits it.
type Lock struct {
	ID        string
	ExpiresAt time.Time
}

// Held answers the lock that still holds at now, or nil: an expired lock is no lock.
func Held(lock *Lock, now time.Time) *Lock {
	if lock == nil || !now.Before(lock.ExpiresAt) {
		return nil
	}
	return lock
}

// LockConflict is a WOPI lock mismatch. It carries the lock that holds, which the protocol answers
// in X-WOPI-Lock ("" for an unlocked file).
type LockConflict struct {
	Current string
}

func (c LockConflict) Error() string { return "media: the file's lock does not match" }

// TakeLock answers the lock after a Lock or RefreshLock request: taken when the file is unlocked,
// renewed when the same lock is asked for again, refused when another holds it (WOPI Lock).
func TakeLock(held *Lock, requested string, now time.Time, isRefresh bool) (Lock, error) {
	if requested == "" || len(requested) > MaxLockLength {
		return Lock{}, LockConflict{Current: currentID(Held(held, now))}
	}
	current := Held(held, now)
	switch {
	case current == nil && isRefresh:
		return Lock{}, LockConflict{Current: ""}
	case current == nil, current.ID == requested:
		return Lock{ID: requested, ExpiresAt: now.Add(LockDuration)}, nil
	default:
		return Lock{}, LockConflict{Current: current.ID}
	}
}

// Relock answers UnlockAndRelock: the old lock must hold, and the new one replaces it.
func Relock(held *Lock, old, requested string, now time.Time) (Lock, error) {
	current := Held(held, now)
	if current == nil || current.ID != old || requested == "" || len(requested) > MaxLockLength {
		return Lock{}, LockConflict{Current: currentID(current)}
	}
	return Lock{ID: requested, ExpiresAt: now.Add(LockDuration)}, nil
}

// Release answers Unlock: only the lock that holds may be released.
func Release(held *Lock, requested string, now time.Time) error {
	current := Held(held, now)
	if current == nil || current.ID != requested {
		return LockConflict{Current: currentID(current)}
	}
	return nil
}

// MayPut answers whether PutFile may write: under the lock that holds, or - the protocol's one
// exception - into an unlocked file that is still empty.
func MayPut(held *Lock, requested string, size int64, now time.Time) error {
	current := Held(held, now)
	if current == nil {
		if size == 0 {
			return nil
		}
		return LockConflict{Current: ""}
	}
	if current.ID != requested {
		return LockConflict{Current: current.ID}
	}
	return nil
}

func currentID(lock *Lock) string {
	if lock == nil {
		return ""
	}
	return lock.ID
}
