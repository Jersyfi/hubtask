// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package filelink

import (
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Connection is a person's own sign-in to a file store, so that *Link a file* offers the store's
// folders to pick from instead of an address to paste (UC-INT-17): a Nextcloud by its login flow
// and an app password the person granted there, or Microsoft's OneDrive and SharePoint by the
// installation's app with a consent to read files. It is its owner's alone; a link it picks is an
// ordinary link and keeps the store's own permissions.
type Connection struct {
	ID        shared.ID
	TenantID  shared.ID
	AccountID shared.ID
	Kind      ConnectionKind
	// Server is a Nextcloud's https address; empty for Microsoft.
	Server string
	// Login is the store's name for the person: a Nextcloud login, a Microsoft account's address.
	Login     string
	CreatedAt time.Time
	UsedAt    time.Time
}

// ConnectionKind is the store a connection signs in to.
type ConnectionKind string

const (
	ConnectNextcloud ConnectionKind = "NEXTCLOUD"
	ConnectMicrosoft ConnectionKind = "MICROSOFT"
)

// Bounds of a person's connections and of what one folder answers.
const (
	MaxConnections   = 10
	MaxFolderItems   = 500
	MaxLoginBytes    = 320
	MaxServerBytes   = 512
	MaxFolderRefSize = 2048
)

// ParseConnectionKind reads a kind; anything else is refused.
func ParseConnectionKind(value string) (ConnectionKind, error) {
	switch kind := ConnectionKind(strings.ToUpper(strings.TrimSpace(value))); kind {
	case ConnectNextcloud, ConnectMicrosoft:
		return kind, nil
	}
	return "", invalid("/kind", "filestore.kind_unknown")
}

// OAuthKey is the kind's key among the installation's OAuth apps; Nextcloud needs none.
func (k ConnectionKind) OAuthKey() string { return strings.ToLower(string(k)) }

// Scopes are what a Microsoft connection asks for: reading the person's files and the sites they
// reach, an offline grant and who signed in - never mail, never writing.
func (k ConnectionKind) Scopes() []string {
	return []string{"https://graph.microsoft.com/Files.Read.All", "https://graph.microsoft.com/Sites.Read.All",
		"offline_access", "openid", "email"}
}

// CheckServer bounds a Nextcloud's address as a person gives it: https, a host, no credentials,
// no query; the path the instance lives under kept, its own pages and doors cut off.
func CheckServer(address string) (string, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return "", invalid("/server", "filestore.server_required")
	}
	if len(address) > MaxServerBytes {
		return "", invalid("/server", "filestore.server_invalid")
	}
	if !strings.Contains(address, "://") {
		address = "https://" + address
	}
	u, err := url.Parse(address)
	if err != nil || u.Host == "" {
		return "", invalid("/server", "filestore.server_invalid")
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return "", invalid("/server", "filestore.server_not_https")
	}
	if u.User != nil {
		return "", invalid("/server", "filestore.server_invalid")
	}
	p := u.Path
	for _, marker := range []string{"/index.php", "/remote.php", "/apps/", "/ocs/", "/login"} {
		if i := strings.Index(p, marker); i >= 0 {
			p = p[:i]
		}
	}
	return "https://" + strings.ToLower(u.Host) + strings.TrimRight(p, "/"), nil
}

// NewConnection keeps what a store answered when the person granted access.
func NewConnection(id, tenant, account shared.ID, kind ConnectionKind, server, login string, now time.Time) (Connection, error) {
	login = strings.TrimSpace(login)
	if len(login) > MaxLoginBytes {
		login = login[:MaxLoginBytes]
	}
	if kind == ConnectNextcloud {
		checked, err := CheckServer(server)
		if err != nil {
			return Connection{}, err
		}
		if login == "" {
			return Connection{}, invalid("/login", "filestore.login_required")
		}
		server = checked
	} else {
		server = ""
	}
	return Connection{ID: id, TenantID: tenant, AccountID: account, Kind: kind, Server: server, Login: login, CreatedAt: now, UsedAt: now}, nil
}

// Item is one file or folder a connection's folder holds: its reference in the store for the
// next folder or a share, what a person sees, and the address an entry's link keeps.
type Item struct {
	Ref      string
	Name     string
	Folder   bool
	Size     int64
	Address  string
	Provider Provider
	Modified time.Time
}

// CheckFolderRef bounds a folder's reference as a client hands it back: at most MaxFolderRefSize
// bytes, valid text, and for a path no step out of the person's own files.
func CheckFolderRef(ref string) (string, error) {
	if len(ref) > MaxFolderRefSize || !utf8.ValidString(ref) || strings.ContainsRune(ref, 0) {
		return "", invalid("/folder", "filestore.folder_invalid")
	}
	for _, segment := range strings.Split(ref, "/") {
		if segment == ".." {
			return "", invalid("/folder", "filestore.folder_invalid")
		}
	}
	return ref, nil
}
