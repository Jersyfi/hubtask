// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package ldap reads a workspace's LDAP directory for its synchronisation (identity.md §27,
// ADR-0099), and is the only package in this repository that imports an LDAP library. It dials
// through the outbound guard as the IMAP reader does, binds only over TLS - ldaps, or StartTLS
// before the bind - and answers people and groups as the domain reads them, never the server's
// words.
package ldap

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	goldap "github.com/go-ldap/ldap/v3"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/ldap"
	"github.com/Jersyfi/hubtask/infrastructure/httpclient"
)

const (
	dialTimeout = 20 * time.Second
	// sessionTimeout bounds one read end to end: a directory that stops answering holds no worker.
	sessionTimeout = 5 * time.Minute
	requestTimeout = 2 * time.Minute
	pageSize       = 500
)

// Directory reads an LDAP directory.
type Directory struct {
	Guard *httpclient.Guard
	// Dial replaces the guarded dial, for tests that speak to a server in the same process.
	Dial func(ctx context.Context, address string) (net.Conn, error)
	// TLS replaces the client configuration, for those tests' certificates.
	TLS *tls.Config
}

var _ port.Reader = Directory{}

// Probe binds and searches the people's base for one entry.
func (d Directory) Probe(ctx context.Context, connection port.Connection) error {
	conn, err := d.open(ctx, connection)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	dir := connection.Directory
	if _, err := goldap.CompileFilter(dir.UserFilter); err != nil {
		return filterInvalid("/user_filter")
	}
	if dir.SyncsGroups() {
		if _, err := goldap.CompileFilter(dir.GroupFilter); err != nil {
			return filterInvalid("/group_filter")
		}
	}
	_, err = conn.Search(goldap.NewSearchRequest(dir.UserBase, goldap.ScopeWholeSubtree, goldap.NeverDerefAliases,
		1, int(requestTimeout.Seconds()), false, dir.UserFilter, []string{"1.1"}, nil))
	if err != nil && !goldap.IsErrorWithCode(err, goldap.LDAPResultSizeLimitExceeded) {
		return coded(err)
	}
	return nil
}

// Read answers the people and, where configured, the groups.
func (d Directory) Read(ctx context.Context, connection port.Connection) (identity.LdapAnswer, error) {
	conn, err := d.open(ctx, connection)
	if err != nil {
		return identity.LdapAnswer{}, err
	}
	defer func() { _ = conn.Close() }()
	dir := connection.Directory
	people, err := search(conn, dir.UserBase, dir.UserFilter, []string{dir.IDAttribute, dir.MailAttribute, dir.NameAttribute})
	if err != nil {
		return identity.LdapAnswer{}, err
	}
	answer := identity.LdapAnswer{People: peopleOf(people, dir)}
	if dir.SyncsGroups() {
		groups, err := search(conn, dir.GroupBase, dir.GroupFilter, []string{dir.IDAttribute, dir.GroupNameAttribute, dir.MemberAttribute})
		if err != nil {
			return identity.LdapAnswer{}, err
		}
		answer.Groups = groupsOf(groups, dir)
	}
	return answer, nil
}

func search(conn *goldap.Conn, base, filter string, attributes []string) ([]*goldap.Entry, error) {
	result, err := conn.SearchWithPaging(goldap.NewSearchRequest(base, goldap.ScopeWholeSubtree, goldap.NeverDerefAliases,
		identity.MaxLdapEntries+1, int(requestTimeout.Seconds()), false, filter, attributes, nil), pageSize)
	if goldap.IsErrorWithCode(err, goldap.LDAPResultSizeLimitExceeded) || (result != nil && len(result.Entries) > identity.MaxLdapEntries) {
		return nil, shared.ErrConflict.WithDetail("directory.ldap_answer_too_large")
	}
	if goldap.IsErrorWithCode(err, goldap.LDAPResultFilterError) {
		return nil, filterInvalid("")
	}
	if err != nil {
		return nil, coded(err)
	}
	return result.Entries, nil
}

func peopleOf(entries []*goldap.Entry, dir identity.LdapDirectory) []identity.LdapPerson {
	people := make([]identity.LdapPerson, 0, len(entries))
	for _, entry := range entries {
		people = append(people, identity.LdapPerson{
			DN: entry.DN, ID: identifierOf(entry, dir.IDAttribute),
			Email: strings.TrimSpace(entry.GetAttributeValue(dir.MailAttribute)),
			Name:  strings.TrimSpace(entry.GetAttributeValue(dir.NameAttribute)),
		})
	}
	return people
}

func groupsOf(entries []*goldap.Entry, dir identity.LdapDirectory) []identity.LdapGroup {
	groups := make([]identity.LdapGroup, 0, len(entries))
	for _, entry := range entries {
		groups = append(groups, identity.LdapGroup{
			MemberNames: identifierOf(entry, dir.IDAttribute), Name: strings.TrimSpace(entry.GetAttributeValue(dir.GroupNameAttribute)),
			MemberDNs: entry.GetAttributeValues(dir.MemberAttribute),
		})
	}
	return groups
}

// identifierOf is the identifier attribute as text: Active Directory's objectGUID is sixteen bytes,
// which are written as hex rather than stored as bytes no column holds.
func identifierOf(entry *goldap.Entry, attribute string) string {
	raw := entry.GetRawAttributeValue(attribute)
	if len(raw) == 0 {
		return ""
	}
	if !utf8.Valid(raw) || strings.ContainsFunc(string(raw), func(r rune) bool { return r < 0x20 }) {
		return hex.EncodeToString(raw)
	}
	return strings.TrimSpace(string(raw))
}

// open dials, secures the connection and binds. Nothing is sent before TLS stands: an ldap URL is
// upgraded by StartTLS first, and a directory that refuses it is unreachable, not read in the clear.
func (d Directory) open(ctx context.Context, connection port.Connection) (*goldap.Conn, error) {
	dir := connection.Directory
	if connection.Password.IsEmpty() {
		return nil, shared.ErrValidation.WithDetail("directory.ldap_refused").
			WithFields(shared.FieldError{Path: "/bind_password", Code: "directory.ldap_refused"})
	}
	raw, err := d.dial(ctx, dir.Address(), dir.Host())
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(sessionTimeout)
	if until, ok := ctx.Deadline(); ok && until.Before(deadline) {
		deadline = until
	}
	_ = raw.SetDeadline(deadline)
	config := d.tlsConfig(dir.Host())
	var conn *goldap.Conn
	if dir.ImplicitTLS() {
		secured := tls.Client(raw, config)
		if err := secured.HandshakeContext(ctx); err != nil {
			_ = raw.Close()
			return nil, unreachable(err)
		}
		conn = goldap.NewConn(secured, true)
	} else {
		conn = goldap.NewConn(raw, false)
	}
	conn.Start()
	conn.SetTimeout(requestTimeout)
	if !dir.ImplicitTLS() {
		if err := conn.StartTLS(config); err != nil {
			_ = conn.Close()
			return nil, unreachable(err)
		}
	}
	if err := conn.Bind(dir.BindDN, connection.Password.Reveal()); err != nil {
		_ = conn.Close()
		return nil, coded(err)
	}
	return conn, nil
}

func (d Directory) tlsConfig(host string) *tls.Config {
	if d.TLS != nil {
		config := d.TLS.Clone()
		config.ServerName = host
		return config
	}
	return &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
}

func (d Directory) dial(ctx context.Context, address, host string) (net.Conn, error) {
	if d.Dial != nil {
		conn, err := d.Dial(ctx, address)
		if err != nil {
			return nil, unreachable(err)
		}
		return conn, nil
	}
	if d.Guard == nil {
		return nil, shared.ErrInternal.WithDetail("directory.ldap_unwired")
	}
	// The checks GuardedClient makes for HTTP, by hand because LDAP is not HTTP: the allowlist,
	// resolution, and the address at dial time, which cannot be raced (rule 6).
	if err := d.Guard.CheckHost(host); err != nil {
		return nil, err
	}
	if _, err := d.Guard.Resolve(ctx, host); err != nil {
		return nil, err
	}
	dialer := &net.Dialer{
		Timeout: dialTimeout,
		Control: func(network, address string, _ syscall.RawConn) error {
			return d.Guard.CheckControl(network, address)
		},
	}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, unreachable(err)
	}
	return conn, nil
}

func filterInvalid(path string) error {
	refusal := shared.ErrValidation.WithDetail("directory.ldap_filter_invalid")
	if path != "" {
		refusal = refusal.WithFields(shared.FieldError{Path: path, Code: "directory.ldap_filter_invalid"})
	}
	return refusal
}

// coded turns the directory's answer into a code: a result the directory gave is a refusal, a
// connection that broke is unreachable.
func coded(err error) error {
	var answer *goldap.Error
	if errors.As(err, &answer) && answer.ResultCode < goldap.ErrorNetwork {
		return shared.ErrValidation.WithDetail("directory.ldap_refused").
			WithFields(shared.FieldError{Path: "/bind_dn", Code: "directory.ldap_refused"}).
			WithCause(fmt.Errorf("ldap result %d", answer.ResultCode))
	}
	return unreachable(err)
}

func unreachable(err error) error {
	var known *shared.Error
	if errors.As(err, &known) {
		return err
	}
	return shared.ErrUnavailable.WithDetail("directory.ldap_unreachable").WithCause(fmt.Errorf("ldap: %w", err))
}
