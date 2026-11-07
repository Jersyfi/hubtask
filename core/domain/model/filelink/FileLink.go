// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package filelink is a file on an entry that lives in a file store (UC-INT-14): a Nextcloud,
// SharePoint, OneDrive or WebDAV file or folder linked by its address, never copied. The file stays
// where it is and keeps its own permissions; the entry keeps the address, a name and the store it
// is in, so that a reader opens it where it lives.
package filelink

import (
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Provider is the file store an address is in.
type Provider string

const (
	Nextcloud  Provider = "NEXTCLOUD"
	SharePoint Provider = "SHAREPOINT"
	OneDrive   Provider = "ONEDRIVE"
	WebDAV     Provider = "WEBDAV"
)

var providers = [...]Provider{Nextcloud, SharePoint, OneDrive, WebDAV}

// Providers is every store, in the order a screen offers them.
func Providers() []Provider { return providers[:] }

// Valid reports whether the provider names a store.
func (p Provider) Valid() bool {
	for _, known := range providers {
		if p == known {
			return true
		}
	}
	return false
}

// Bounds of what is kept.
const (
	MaxNameRunes     = 200
	MaxAddressBytes  = 2048
	MaxLinksPerEntry = 50
)

// Link is one file or folder an entry names in a store.
type Link struct {
	ID       shared.ID
	TenantID shared.ID
	ItemID   shared.ID
	Provider Provider
	Address  string
	Name     string
	AddedBy  shared.ID
	AddedAt  time.Time
}

func invalid(path, code string, params ...string) error {
	err := shared.ErrValidation.WithDetail(code).WithFields(shared.FieldError{Path: path, Code: code})
	if len(params) == 2 {
		err = err.WithParams(map[string]string{params[0]: params[1]})
	}
	return err
}

// Detect answers the store an address is in by its shape: SharePoint and OneDrive by their hosts,
// Nextcloud by the paths its share links, its file view and its WebDAV door take. Anything else
// is a WebDAV store, the one shape every store answers.
func Detect(u *url.URL) Provider {
	host := strings.ToLower(u.Hostname())
	switch {
	case host == "onedrive.live.com" || host == "1drv.ms" || strings.HasSuffix(host, "-my.sharepoint.com"):
		return OneDrive
	case strings.HasSuffix(host, ".sharepoint.com"):
		return SharePoint
	}
	p := u.Path
	for _, marker := range []string{"/remote.php/dav/", "/remote.php/webdav/", "/index.php/s/", "/index.php/f/", "/apps/files/"} {
		if strings.Contains(p, marker) {
			return Nextcloud
		}
	}
	if strings.HasPrefix(p, "/s/") || strings.HasPrefix(p, "/f/") {
		return Nextcloud
	}
	return WebDAV
}

// defaultName is what an address is called when the person names it not: its last path segment,
// unescaped, or its host.
func defaultName(u *url.URL) string {
	segment := path.Base(strings.TrimRight(u.Path, "/"))
	if segment == "." || segment == "/" || segment == "" {
		return u.Hostname()
	}
	if unescaped, err := url.PathUnescape(segment); err == nil {
		segment = unescaped
	}
	return segment
}

// Check bounds a link as a person gives it: an https address with a host and no credentials in
// it, a known store or one detected from the address, a name of at most MaxNameRunes or one taken
// from the address. A `davs://` address is the https address of the same WebDAV door.
func Check(address string, provider Provider, name string) (string, Provider, string, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return "", "", "", invalid("/address", "filelink.address_required")
	}
	if len(address) > MaxAddressBytes {
		return "", "", "", invalid("/address", "filelink.address_too_long", "maximum", strconv.Itoa(MaxAddressBytes))
	}
	u, err := url.Parse(address)
	if err != nil || u.Host == "" {
		return "", "", "", invalid("/address", "filelink.address_invalid")
	}
	if strings.EqualFold(u.Scheme, "davs") {
		u.Scheme = "https"
		if provider == "" {
			provider = WebDAV
		}
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return "", "", "", invalid("/address", "filelink.address_not_https")
	}
	if u.User != nil {
		return "", "", "", invalid("/address", "filelink.address_has_credentials")
	}
	u.Scheme = "https"
	if provider == "" {
		provider = Detect(u)
	}
	if !provider.Valid() {
		return "", "", "", invalid("/provider", "filelink.provider_unknown")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = defaultName(u)
	}
	if utf8.RuneCountInString(name) > MaxNameRunes {
		return "", "", "", invalid("/name", "filelink.name_too_long", "maximum", strconv.Itoa(MaxNameRunes))
	}
	return u.String(), provider, name, nil
}
