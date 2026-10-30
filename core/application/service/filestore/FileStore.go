// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package filestore is the use cases of a person's file store connections (UC-INT-17): they sign
// in to their own Nextcloud by its login flow, or to Microsoft's OneDrive and SharePoint with the
// installation's app and a consent to read files, and *Link a file* then offers the store's
// folders to pick from and, on a Nextcloud, a read-only share link. The link an entry keeps is an
// ordinary file link (UC-INT-14), judged on the entry by its own use case.
//
// Everything here is the owner's: nobody lists, browses with or disconnects somebody else's
// connection, an administrator included, because a connection is a credential to that person's
// own files.
package filestore

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/filestore"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/filelink"
	maildomain "github.com/Jersyfi/hubtask/core/domain/model/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	storeport "github.com/Jersyfi/hubtask/core/port/filestore"
	mailboxport "github.com/Jersyfi/hubtask/core/port/mailbox"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// The use case names.
const (
	ListFileStoreConnectionsName    = "ListFileStoreConnections"
	ConnectFileStoreName            = "ConnectFileStore"
	CompleteFileStoreConnectionName = "CompleteFileStoreConnection"
	DisconnectFileStoreName         = "DisconnectFileStore"
	BrowseFileStoreName             = "BrowseFileStore"
	ShareFileStoreItemName          = "ShareFileStoreItem"
)

// The stable audit codes (audit.md §2). A file's name or address never reaches the trail
// (rule 10): it says what the file holds.
const (
	ListedAction       audit.Action = "filestore.listed"
	StartedAction      audit.Action = "filestore.connection_started"
	ConnectedAction    audit.Action = "filestore.connected"
	DisconnectedAction audit.Action = "filestore.disconnected"
	BrowsedAction      audit.Action = "filestore.browsed"
	SharedAction       audit.Action = "filestore.shared"
	connectionTarget                = "file_store_connection"
	readScope                       = "items:read"
	writeScope                      = "items:write"
	exemptReason                    = "a file store connection is its owner's sign-in to their own files; it changes no entry."

	stateBytes    = 32
	verifierBytes = 32
	pendingTTL    = 10 * time.Minute
	callLimit     = 20 * time.Second
)

// App is an OAuth client the installation registered at Microsoft, the mailboxes' own.
type App struct {
	ClientID     string
	ClientSecret secret.Secret
}

// Sealer seals and opens a grant under its connection's purpose.
type Sealer interface {
	Seal(ctx context.Context, plaintext secret.Secret, purpose crypto.Purpose) (crypto.Sealed, error)
	Open(ctx context.Context, sealed crypto.Sealed, purpose crypto.Purpose) (secret.Secret, error)
}

// Entropy draws unguessable bytes.
type Entropy interface {
	Bytes(n int) ([]byte, error)
}

// Connections is what every file store use case shares.
type Connections struct {
	Store      repository.Store
	Files      storeport.Store
	Authorizer mailboxport.Authorizer
	Encryptor  Sealer
	Entropy    Entropy
	// Apps are the installation's apps by provider key; Microsoft without one is not offered.
	Apps map[string]App
	// RedirectURL is the web UI's page Microsoft sends the browser back to.
	RedirectURL string
	Audit       audit.Sink
	UnitOfWork  persistence.UnitOfWork
	Clock       clock.Clock
	IDs         clock.IDGenerator
}

func secretPurpose(id shared.ID) crypto.Purpose {
	return crypto.Purpose("filestore.secret:" + id.String())
}

func pendingPurpose(id shared.ID) crypto.Purpose {
	return crypto.Purpose("filestore.pending:" + id.String())
}

func stateHash(state string) []byte {
	sum := sha256.Sum256([]byte(state))
	return sum[:]
}

func (c Connections) microsoft() (maildomain.OAuthClient, secret.Secret, error) {
	kind := filelink.ConnectMicrosoft
	app, ok := c.Apps[kind.OAuthKey()]
	preset, known := maildomain.OAuthPresetNamed(kind.OAuthKey())
	if !ok || !known || app.ClientID == "" || c.RedirectURL == "" {
		return maildomain.OAuthClient{}, secret.Secret{}, shared.ErrUnavailable.WithDetail("filestore.kind_not_offered")
	}
	return maildomain.OAuthClient{
		Provider: kind.OAuthKey(), AuthorizeURL: preset.AuthorizeURL, TokenURL: preset.TokenURL,
		Scopes: kind.Scopes(), ClientID: app.ClientID, Installation: true,
	}, app.ClientSecret, nil
}

// Offered answers the stores this installation connects to: a Nextcloud always, its login flow
// needing no app; Microsoft with the installation's app.
func (c Connections) Offered() []string {
	out := []string{string(filelink.ConnectNextcloud)}
	if _, _, err := c.microsoft(); err == nil {
		out = append(out, string(filelink.ConnectMicrosoft))
	}
	return out
}

func (c Connections) record(ctx context.Context, actor appshared.ActorContext, action audit.Action, id shared.ID, changes ...audit.Change) {
	if c.Audit == nil {
		return
	}
	_ = c.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: c.Clock.Now(), Action: action,
		Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: connectionTarget, TargetID: id, Changes: audit.Changes(changes...),
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
	})
}

func kindChange(kind filelink.ConnectionKind) audit.Change {
	return audit.Change{Field: "kind", Classification: audit.Open, To: string(kind)}
}

func (c Connections) draw(n int) (string, error) {
	drawn, err := c.Entropy.Bytes(n)
	if err != nil {
		return "", shared.ErrInternal.WithDetail("filestore.unavailable").WithCause(err)
	}
	return base64.RawURLEncoding.EncodeToString(drawn), nil
}

// Output is a connection on the wire: never its grant.
func Output(conn filelink.Connection) usecase.Output {
	out := usecase.Output{
		"id": conn.ID.String(), "kind": string(conn.Kind), "login": conn.Login,
		"created_at": conn.CreatedAt.UTC(), "used_at": conn.UsedAt.UTC(),
	}
	if conn.Server != "" {
		out["server"] = conn.Server
	}
	return out
}

func itemOutput(item filelink.Item) usecase.Output {
	out := usecase.Output{"ref": item.Ref, "name": item.Name, "folder": item.Folder}
	if item.Address != "" {
		out["address"] = item.Address
	}
	if item.Provider != "" {
		out["provider"] = string(item.Provider)
	}
	if !item.Folder {
		out["size"] = item.Size
	}
	if !item.Modified.IsZero() {
		out["modified_at"] = item.Modified.UTC()
	}
	return out
}

// owned answers the caller's own connection with its grant opened; anybody else's is unknown.
func (c Connections) owned(ctx context.Context, actor appshared.ActorContext, id shared.ID) (filelink.Connection, secret.Secret, error) {
	var (
		conn   filelink.Connection
		sealed crypto.Sealed
	)
	err := c.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		conn, sealed, err = c.Store.Find(ctx, id)
		return err
	})
	if err != nil {
		return filelink.Connection{}, secret.Secret{}, err
	}
	if conn.AccountID != actor.AccountID {
		return filelink.Connection{}, secret.Secret{}, shared.ErrNotFound.WithDetail("filestore.unknown")
	}
	grant, err := c.Encryptor.Open(ctx, sealed, secretPurpose(conn.ID))
	if err != nil {
		return filelink.Connection{}, secret.Secret{}, shared.ErrForbidden.WithDetail("filestore.grant_revoked").WithCause(err)
	}
	return conn, grant, nil
}

// access answers what a call into the store carries: a Nextcloud's app password, or a Microsoft
// access token drawn now from the refresh token, a rotated one sealed again.
func (c Connections) access(ctx context.Context, actor appshared.ActorContext, conn filelink.Connection, grant secret.Secret) (storeport.Access, error) {
	access := storeport.Access{Kind: conn.Kind, Server: conn.Server, Login: conn.Login, Secret: grant}
	if conn.Kind != filelink.ConnectMicrosoft {
		return access, nil
	}
	client, clientSecret, err := c.microsoft()
	if err != nil {
		return storeport.Access{}, err
	}
	token, err := c.Authorizer.Refresh(ctx, client, clientSecret, grant)
	if err != nil {
		return storeport.Access{}, shared.ErrForbidden.WithDetail("filestore.grant_revoked").WithCause(err)
	}
	if !token.Refresh.IsEmpty() && token.Refresh.Reveal() != grant.Reveal() {
		if resealed, err := c.Encryptor.Seal(ctx, token.Refresh, secretPurpose(conn.ID)); err == nil {
			_ = c.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				return c.Store.Reseal(ctx, conn.ID, resealed)
			})
		}
	}
	access.Secret = token.Access
	return access, nil
}

func (c Connections) touch(ctx context.Context, actor appshared.ActorContext, id shared.ID) {
	_ = c.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		return c.Store.Touch(ctx, id, c.Clock.Now())
	})
}

func connectionInput() usecase.Field {
	return usecase.Field{Name: "connection_id", Kind: usecase.KindID, Required: true, Description: "The caller's own file store connection."}
}

// ListFileStoreConnections answers the caller's own connections.
type ListFileStoreConnections struct{ Connections Connections }

// Descriptor is the catalogue entry.
func (h ListFileStoreConnections) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListFileStoreConnectionsName,
		Summary: "Lists the caller's own file store connections - their sign-ins to a Nextcloud or to Microsoft's " +
			"OneDrive and SharePoint - with the login and when each was last used, and the stores this installation " +
			"connects to. Never anybody else's.",
		SideEffects: "None. Reads only.", TokenScope: readScope, ReadOnly: true,
		Audit:    usecase.AuditDeclaration{Action: ListedAction, TargetType: connectionTarget, Severity: audit.SeverityInfo},
		Activity: usecase.ActivityDeclaration{Exempt: exemptReason},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			if err := actor.RequireScope(readScope); err != nil {
				return nil, err
			}
			rows := []usecase.Output{}
			err := h.Connections.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				list, err := h.Connections.Store.List(ctx, actor.AccountID)
				for _, conn := range list {
					rows = append(rows, Output(conn))
				}
				return err
			})
			return usecase.Output{"data": rows, "kinds": h.Connections.Offered()}, err
		}),
	}
}

// ConnectFileStore starts a connection.
type ConnectFileStore struct{ Connections Connections }

// Descriptor is the catalogue entry.
func (h ConnectFileStore) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ConnectFileStoreName,
		Summary: "Starts connecting the caller to their own file store: for a Nextcloud, its login flow - answers " +
			"the Nextcloud's page where they grant access and a state to finish with; for Microsoft, the sign-in " +
			"page where they consent to reading their files. The connection waits ten minutes.",
		SideEffects: "Remembers the waiting connection and writes an audit entry.", TokenScope: writeScope,
		Input: []usecase.Field{
			{Name: "kind", Kind: usecase.KindString, Required: true, Enum: []string{string(filelink.ConnectNextcloud), string(filelink.ConnectMicrosoft)}, Description: "The store."},
			{Name: "server", Kind: usecase.KindString, Description: "A Nextcloud's https address; required for a Nextcloud."},
		},
		Audit:    usecase.AuditDeclaration{Required: true, Action: StartedAction, TargetType: connectionTarget, Severity: audit.SeverityInfo},
		Activity: usecase.ActivityDeclaration{Exempt: exemptReason},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

func (h ConnectFileStore) invoke(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	c := h.Connections
	if err := actor.RequireScope(writeScope); err != nil {
		return nil, err
	}
	kind, err := filelink.ParseConnectionKind(in.String("kind"))
	if err != nil {
		return nil, err
	}
	now := c.Clock.Now()
	state, err := c.draw(stateBytes)
	if err != nil {
		return nil, err
	}
	pending := repository.Pending{ID: c.IDs.NewID(), AccountID: actor.AccountID, Kind: kind, ExpiresAt: now.Add(pendingTTL)}
	out := usecase.Output{"expires_at": pending.ExpiresAt.UTC()}
	var hidden secret.Secret
	if kind == filelink.ConnectNextcloud {
		if pending.Server, err = filelink.CheckServer(in.String("server")); err != nil {
			return nil, err
		}
		call, cancel := context.WithTimeout(ctx, callLimit)
		login, err := c.Files.BeginLogin(call, pending.Server)
		cancel()
		if err != nil {
			return nil, err
		}
		pending.PollEndpoint, hidden = login.PollEndpoint, login.PollToken
		out["authorize_url"], out["state"] = login.LoginURL, state
	} else {
		client, _, err := c.microsoft()
		if err != nil {
			return nil, err
		}
		verifier, err := c.draw(verifierBytes)
		if err != nil {
			return nil, err
		}
		hidden = secret.New(verifier)
		out["authorize_url"] = client.AuthorizationURL(c.RedirectURL, state, verifier, "")
	}
	if pending.Secret, err = c.Encryptor.Seal(ctx, hidden, pendingPurpose(pending.ID)); err != nil {
		return nil, err
	}
	err = c.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if _, err := c.Store.DeleteExpired(ctx, now); err != nil {
			return err
		}
		count, err := c.Store.Count(ctx, actor.AccountID)
		if err != nil {
			return err
		}
		if count >= filelink.MaxConnections {
			return shared.ErrConflict.WithDetail("filestore.connections_full")
		}
		return c.Store.Await(ctx, pending, stateHash(state))
	})
	if err != nil {
		return nil, err
	}
	c.record(ctx, actor, StartedAction, pending.ID, kindChange(kind))
	return out, nil
}

// CompleteFileStoreConnection finishes connecting.
type CompleteFileStoreConnection struct{ Connections Connections }

// Descriptor is the catalogue entry.
func (h CompleteFileStoreConnection) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CompleteFileStoreConnectionName,
		Summary: "Finishes connecting a file store: for a Nextcloud, fetches the app password the caller granted " +
			"there - refused as not yet granted while they have not, the connection still waiting; for Microsoft, " +
			"trades the code the browser came back with. The grant is kept sealed; signing in again as the same " +
			"login replaces it.",
		SideEffects: "Stores the connection with its sealed grant and writes an audit entry.", TokenScope: writeScope,
		Input: []usecase.Field{
			{Name: "state", Kind: usecase.KindString, Required: true, Description: "The state of the waiting connection."},
			{Name: "code", Kind: usecase.KindString, Description: "Microsoft's authorization code."},
			{Name: "error", Kind: usecase.KindString, Description: "Microsoft's refusal, when the person said no."},
		},
		Audit:    usecase.AuditDeclaration{Required: true, Action: ConnectedAction, TargetType: connectionTarget, Severity: audit.SeverityInfo},
		Activity: usecase.ActivityDeclaration{Exempt: exemptReason},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

func (h CompleteFileStoreConnection) invoke(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	c := h.Connections
	if err := actor.RequireScope(writeScope); err != nil {
		return nil, err
	}
	state := in.String("state")
	if state == "" {
		return nil, shared.ErrValidation.WithDetail("filestore.state_unknown")
	}
	var pending repository.Pending
	if err := c.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		pending, err = c.Store.Pending(ctx, stateHash(state))
		return err
	}); err != nil {
		return nil, err
	}
	if pending.AccountID != actor.AccountID {
		return nil, shared.ErrNotFound.WithDetail("filestore.state_unknown")
	}
	forget := func() error {
		return c.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error { return c.Store.Forget(ctx, pending.ID) })
	}
	now := c.Clock.Now()
	if !now.Before(pending.ExpiresAt) {
		_ = forget()
		return nil, shared.ErrValidation.WithDetail("filestore.expired")
	}
	hidden, err := c.Encryptor.Open(ctx, pending.Secret, pendingPurpose(pending.ID))
	if err != nil {
		return nil, err
	}
	call, cancel := context.WithTimeout(ctx, callLimit)
	defer cancel()
	var (
		login string
		grant secret.Secret
	)
	if pending.Kind == filelink.ConnectNextcloud {
		answered, granted, err := c.Files.PollLogin(call, pending.PollEndpoint, hidden)
		if err != nil {
			return nil, err
		}
		if !granted {
			// The person has not granted yet: the connection keeps waiting for the next try.
			return nil, shared.ErrConflict.WithDetail("filestore.not_granted_yet")
		}
		login, grant = answered.Login, answered.AppPassword
	} else {
		// A Microsoft state is good for one try, whatever it brings.
		if err := forget(); err != nil {
			return nil, err
		}
		code := in.String("code")
		if in.String("error") != "" || code == "" {
			return nil, shared.ErrValidation.WithDetail("filestore.denied")
		}
		client, clientSecret, err := c.microsoft()
		if err != nil {
			return nil, err
		}
		token, err := c.Authorizer.Exchange(call, client, clientSecret, code, hidden.Reveal(), c.RedirectURL)
		if err != nil {
			return nil, shared.ErrValidation.WithDetail("filestore.denied").WithCause(err)
		}
		if token.Refresh.IsEmpty() {
			return nil, shared.ErrValidation.WithDetail("filestore.no_refresh")
		}
		login, grant = token.Email, token.Refresh
	}
	conn, err := filelink.NewConnection(c.IDs.NewID(), actor.TenantID, actor.AccountID, pending.Kind, pending.Server, login, now)
	if err != nil {
		return nil, err
	}
	err = c.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := c.Store.Forget(ctx, pending.ID); err != nil {
			return err
		}
		sealed, err := c.Encryptor.Seal(ctx, grant, secretPurpose(conn.ID))
		if err != nil {
			return err
		}
		stored, err := c.Store.Connect(ctx, conn, sealed)
		if err != nil {
			return err
		}
		if stored != conn.ID {
			// Signed in again as the same login: the connection keeps its identity and takes the
			// new grant, sealed under that identity's purpose.
			conn.ID = stored
			resealed, err := c.Encryptor.Seal(ctx, grant, secretPurpose(stored))
			if err != nil {
				return err
			}
			return c.Store.Reseal(ctx, stored, resealed)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	c.record(ctx, actor, ConnectedAction, conn.ID, kindChange(conn.Kind))
	return Output(conn), nil
}

// DisconnectFileStore takes a connection off.
type DisconnectFileStore struct{ Connections Connections }

// Descriptor is the catalogue entry.
func (h DisconnectFileStore) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: DisconnectFileStoreName,
		Summary: "Takes one of the caller's own file store connections off: its grant is forgotten, and a " +
			"Nextcloud's app password given back. The files linked through it stay linked; they open in the store " +
			"as before.",
		SideEffects: "Removes the connection and writes an audit entry.", TokenScope: writeScope,
		Input:    []usecase.Field{connectionInput()},
		Audit:    usecase.AuditDeclaration{Required: true, Action: DisconnectedAction, TargetType: connectionTarget, Severity: audit.SeverityInfo},
		Activity: usecase.ActivityDeclaration{Exempt: exemptReason},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			c := h.Connections
			if err := actor.RequireScope(writeScope); err != nil {
				return nil, err
			}
			id, err := in.ID("connection_id")
			if err != nil {
				return nil, err
			}
			conn, grant, err := c.owned(ctx, actor, id)
			if err != nil && !errors.Is(err, shared.ErrForbidden) {
				return nil, err
			}
			if err == nil && conn.Kind == filelink.ConnectNextcloud {
				call, cancel := context.WithTimeout(ctx, callLimit)
				_ = c.Files.Revoke(call, storeport.Access{Kind: conn.Kind, Server: conn.Server, Login: conn.Login, Secret: grant})
				cancel()
			}
			err = c.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				found, _, err := c.Store.Find(ctx, id)
				if err != nil {
					return err
				}
				if found.AccountID != actor.AccountID {
					return shared.ErrNotFound.WithDetail("filestore.unknown")
				}
				return c.Store.Disconnect(ctx, id)
			})
			if err != nil {
				return nil, err
			}
			c.record(ctx, actor, DisconnectedAction, id)
			return usecase.Output{}, nil
		}),
	}
}

// BrowseFileStore answers a folder of the caller's own store.
type BrowseFileStore struct{ Connections Connections }

// Descriptor is the catalogue entry.
func (h BrowseFileStore) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: BrowseFileStoreName,
		Summary: "Answers one folder of the caller's own file store through their connection: its files and " +
			"folders with name, size and the address an entry's link keeps, at most 500. The empty folder is the " +
			"top: a Nextcloud's own files; Microsoft's OneDrive and the SharePoint sites the caller follows. Reads " +
			"names, never content.",
		SideEffects: "None in the workspace. Reads only.", TokenScope: readScope, ReadOnly: true,
		Input: []usecase.Field{
			connectionInput(),
			{Name: "folder", Kind: usecase.KindString, Description: "A folder's reference as a previous answer gave it; the top when empty."},
		},
		Audit:    usecase.AuditDeclaration{Action: BrowsedAction, TargetType: connectionTarget, Severity: audit.SeverityInfo},
		Activity: usecase.ActivityDeclaration{Exempt: exemptReason},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			c := h.Connections
			if err := actor.RequireScope(readScope); err != nil {
				return nil, err
			}
			id, err := in.ID("connection_id")
			if err != nil {
				return nil, err
			}
			folder, err := filelink.CheckFolderRef(strings.TrimSpace(in.String("folder")))
			if err != nil {
				return nil, err
			}
			conn, grant, err := c.owned(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			call, cancel := context.WithTimeout(ctx, callLimit)
			defer cancel()
			access, err := c.access(call, actor, conn, grant)
			if err != nil {
				return nil, err
			}
			items, err := c.Files.Browse(call, access, folder)
			if err != nil {
				return nil, err
			}
			c.touch(ctx, actor, conn.ID)
			rows := make([]usecase.Output, 0, len(items))
			for _, item := range items {
				rows = append(rows, itemOutput(item))
			}
			return usecase.Output{"data": rows, "folder": folder}, nil
		}),
	}
}

// ShareFileStoreItem makes a read-only share link in the caller's own Nextcloud.
type ShareFileStoreItem struct{ Connections Connections }

// Descriptor is the catalogue entry.
func (h ShareFileStoreItem) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ShareFileStoreItemName,
		Summary: "Makes a read-only share link for a file or folder in the caller's own Nextcloud, through their " +
			"connection, for an entry's readers who have no account there. Anybody holding the link can open the " +
			"file; the caller takes the share back in Nextcloud. Microsoft's files are linked as they are.",
		SideEffects: "Creates a public, read-only share in the caller's Nextcloud and writes an audit entry.",
		TokenScope:  writeScope,
		Input: []usecase.Field{
			connectionInput(),
			{Name: "ref", Kind: usecase.KindString, Required: true, Description: "The file's or folder's reference as a folder's answer gave it."},
		},
		Audit:    usecase.AuditDeclaration{Required: true, Action: SharedAction, TargetType: connectionTarget, Severity: audit.SeverityInfo},
		Activity: usecase.ActivityDeclaration{Exempt: exemptReason},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			c := h.Connections
			if err := actor.RequireScope(writeScope); err != nil {
				return nil, err
			}
			id, err := in.ID("connection_id")
			if err != nil {
				return nil, err
			}
			ref, err := filelink.CheckFolderRef(strings.TrimSpace(in.String("ref")))
			if err != nil || ref == "" || ref == "/" {
				return nil, shared.ErrValidation.WithDetail("filestore.folder_invalid")
			}
			conn, grant, err := c.owned(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			if conn.Kind != filelink.ConnectNextcloud {
				return nil, shared.ErrValidation.WithDetail("filestore.share_not_offered")
			}
			call, cancel := context.WithTimeout(ctx, callLimit)
			defer cancel()
			address, err := c.Files.Share(call, storeport.Access{Kind: conn.Kind, Server: conn.Server, Login: conn.Login, Secret: grant}, ref)
			if err != nil {
				return nil, err
			}
			c.touch(ctx, actor, conn.ID)
			c.record(ctx, actor, SharedAction, conn.ID, kindChange(conn.Kind))
			return usecase.Output{"address": address, "provider": string(filelink.Nextcloud)}, nil
		}),
	}
}
