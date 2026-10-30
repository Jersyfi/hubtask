// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package ldap is how a workspace's LDAP directory is read (UC-ID-31, identity.md §27): bind as the
// configured account over TLS, search the people and the groups, and answer them as the domain
// reads them. One adapter implements it, the only place the LDAP library is imported (ADR-0099).
package ldap

import (
	"context"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// Connection is a directory with its bind password, opened for one read and forgotten after it.
type Connection struct {
	Directory identity.LdapDirectory
	Password  secret.Secret
}

// Reader reads a directory. Refusals are coded, never the server's words: a bind or a search the
// directory refuses is `directory.ldap_refused`, a directory that cannot be reached or does not
// speak TLS `directory.ldap_unreachable`, a filter it cannot read `directory.ldap_filter_invalid`,
// and an answer over identity.MaxLdapEntries `directory.ldap_answer_too_large`.
type Reader interface {
	// Probe binds and searches the people's base for one entry, and lets go again.
	Probe(ctx context.Context, connection Connection) error
	// Read answers every person and, where the directory reads groups, every group.
	Read(ctx context.Context, connection Connection) (identity.LdapAnswer, error)
}
