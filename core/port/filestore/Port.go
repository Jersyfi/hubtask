// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package filestore is the port a file store connection speaks through (UC-INT-17): a
// Nextcloud's login flow, its folders by WebDAV and its share links by OCS, and Microsoft's
// OneDrive and SharePoint folders by Graph.
package filestore

import (
	"context"

	"github.com/Jersyfi/hubtask/core/domain/model/filelink"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// Login is a Nextcloud login flow just begun: the page the person grants access on, and the
// poll token and address the grant is fetched with once they did.
type Login struct {
	LoginURL     string
	PollEndpoint string
	PollToken    secret.Secret
}

// Grant is what a Nextcloud answers once the person granted access: its own address, their
// login and an app password for this connection alone.
type Grant struct {
	Server      string
	Login       string
	AppPassword secret.Secret
}

// Access is what a call into a store carries.
type Access struct {
	Kind   filelink.ConnectionKind
	Server string
	Login  string
	// Secret is a Nextcloud's app password, or a Microsoft access token just drawn.
	Secret secret.Secret
}

// Store speaks to the stores. Refusals are coded: a grant the store no longer takes is
// `filestore.grant_revoked`, a store that cannot be reached or refused for now
// `filestore.store_unreachable`, a folder or file it does not have `filestore.item_unknown`.
type Store interface {
	// BeginLogin starts a Nextcloud's login flow v2.
	BeginLogin(ctx context.Context, server string) (Login, error)
	// PollLogin fetches the grant; granted is false while the person has not granted yet.
	PollLogin(ctx context.Context, endpoint string, token secret.Secret) (grant Grant, granted bool, err error)
	// Browse answers a folder's files and folders, at most filelink.MaxFolderItems; the empty
	// reference is the person's own top.
	Browse(ctx context.Context, access Access, folder string) ([]filelink.Item, error)
	// Share answers a read-only share link for a Nextcloud file or folder, made by its owner.
	Share(ctx context.Context, access Access, ref string) (string, error)
	// Revoke gives a Nextcloud's app password back; a store that cannot be reached is no failure
	// of the disconnect.
	Revoke(ctx context.Context, access Access) error
}
