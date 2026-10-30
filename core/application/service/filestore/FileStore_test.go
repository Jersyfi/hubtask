// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package filestore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/filestore"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/filelink"
	maildomain "github.com/Jersyfi/hubtask/core/domain/model/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	storeport "github.com/Jersyfi/hubtask/core/port/filestore"
	mailboxport "github.com/Jersyfi/hubtask/core/port/mailbox"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

var (
	tenant   = shared.ID("00000000-0000-7000-8000-0000000000d1")
	owner    = shared.ID("00000000-0000-7000-8000-0000000000d2")
	stranger = shared.ID("00000000-0000-7000-8000-0000000000d3")
	msID     = shared.ID("00000000-0000-7000-8000-0000000000d4")
	now      = time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)
)

type passthrough struct{}

func (passthrough) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}
func (passthrough) WithinReadOnly(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

type clockAt struct{ now time.Time }

func (c clockAt) Now() time.Time { return c.now }

type counter struct{ n int }

func (c *counter) NewID() shared.ID {
	c.n++
	return shared.ID(fmt.Sprintf("00000000-0000-7000-8000-%012d", c.n))
}

type entropy struct{ n byte }

func (e *entropy) Bytes(n int) ([]byte, error) {
	e.n++
	out := make([]byte, n)
	out[0] = e.n
	return out, nil
}

type plain struct{}

func (plain) Seal(_ context.Context, s secret.Secret, p crypto.Purpose) (crypto.Sealed, error) {
	return crypto.Sealed{KeyID: string(p), Ciphertext: []byte(s.Reveal())}, nil
}
func (plain) Open(_ context.Context, s crypto.Sealed, p crypto.Purpose) (secret.Secret, error) {
	if s.KeyID != string(p) {
		return secret.Secret{}, errors.New("another purpose")
	}
	return secret.New(string(s.Ciphertext)), nil
}

type store struct {
	conns   map[shared.ID]filelink.Connection
	sealed  map[shared.ID]crypto.Sealed
	pending map[string]repository.Pending
}

func (s *store) List(_ context.Context, account shared.ID) ([]filelink.Connection, error) {
	var out []filelink.Connection
	for _, c := range s.conns {
		if c.AccountID == account {
			out = append(out, c)
		}
	}
	return out, nil
}
func (s *store) Count(ctx context.Context, account shared.ID) (int, error) {
	l, _ := s.List(ctx, account)
	return len(l), nil
}
func (s *store) Find(_ context.Context, id shared.ID) (filelink.Connection, crypto.Sealed, error) {
	c, ok := s.conns[id]
	if !ok {
		return c, crypto.Sealed{}, shared.ErrNotFound
	}
	return c, s.sealed[id], nil
}
func (s *store) Connect(_ context.Context, c filelink.Connection, sealed crypto.Sealed) (shared.ID, error) {
	for id, known := range s.conns {
		if known.AccountID == c.AccountID && known.Kind == c.Kind && known.Server == c.Server && known.Login == c.Login {
			s.sealed[id] = sealed
			return id, nil
		}
	}
	s.conns[c.ID], s.sealed[c.ID] = c, sealed
	return c.ID, nil
}
func (s *store) Reseal(_ context.Context, id shared.ID, sealed crypto.Sealed) error {
	s.sealed[id] = sealed
	return nil
}
func (s *store) Touch(context.Context, shared.ID, time.Time) error { return nil }
func (s *store) Disconnect(_ context.Context, id shared.ID) error {
	delete(s.conns, id)
	return nil
}
func (s *store) Await(_ context.Context, p repository.Pending, hash []byte) error {
	s.pending[string(hash)] = p
	return nil
}
func (s *store) Pending(_ context.Context, hash []byte) (repository.Pending, error) {
	p, ok := s.pending[string(hash)]
	if !ok {
		return p, shared.ErrNotFound
	}
	return p, nil
}
func (s *store) Forget(_ context.Context, id shared.ID) error {
	for k, p := range s.pending {
		if p.ID == id {
			delete(s.pending, k)
		}
	}
	return nil
}
func (s *store) DeleteExpired(context.Context, time.Time) (int, error) { return 0, nil }

type files struct {
	granted bool
	revoked []string
	access  []storeport.Access
}

func (f *files) BeginLogin(_ context.Context, server string) (storeport.Login, error) {
	return storeport.Login{LoginURL: server + "/login/v2/flow/x", PollEndpoint: server + "/login/v2/poll", PollToken: secret.New("poll")}, nil
}
func (f *files) PollLogin(_ context.Context, _ string, token secret.Secret) (storeport.Grant, bool, error) {
	if !f.granted || token.Reveal() != "poll" {
		return storeport.Grant{}, false, nil
	}
	return storeport.Grant{Login: "ada", AppPassword: secret.New("app-pw")}, true, nil
}
func (f *files) Browse(_ context.Context, access storeport.Access, folder string) ([]filelink.Item, error) {
	f.access = append(f.access, access)
	return []filelink.Item{{Ref: folder + "/Spec.pdf", Name: "Spec.pdf", Address: "https://cloud.example.org/index.php/f/1", Provider: filelink.Nextcloud}}, nil
}
func (f *files) Share(_ context.Context, access storeport.Access, ref string) (string, error) {
	f.access = append(f.access, access)
	return "https://cloud.example.org/s/abc", nil
}
func (f *files) Revoke(_ context.Context, access storeport.Access) error {
	f.revoked = append(f.revoked, access.Secret.Reveal())
	return nil
}

type authorizer struct{}

func (authorizer) Exchange(_ context.Context, client maildomain.OAuthClient, _ secret.Secret, code, verifier, _ string) (mailboxport.Token, error) {
	if verifier == "" {
		return mailboxport.Token{}, errors.New("no verifier")
	}
	return mailboxport.Token{Access: secret.New("access"), Refresh: secret.New("refresh-" + code), Email: "ada@example.org"}, nil
}
func (authorizer) Refresh(_ context.Context, _ maildomain.OAuthClient, _ secret.Secret, refresh secret.Secret) (mailboxport.Token, error) {
	return mailboxport.Token{Access: secret.New("access-of-" + refresh.Reveal())}, nil
}

type fixture struct {
	conns Connections
	store *store
	files *files
}

func newFixture() fixture {
	f := fixture{store: &store{conns: map[shared.ID]filelink.Connection{}, sealed: map[shared.ID]crypto.Sealed{},
		pending: map[string]repository.Pending{}}, files: &files{}}
	f.conns = Connections{
		Store: f.store, Files: f.files, Authorizer: authorizer{}, Encryptor: plain{}, Entropy: &entropy{},
		Apps:        map[string]App{"microsoft": {ClientID: "client", ClientSecret: secret.New("s")}},
		RedirectURL: "https://hubtask.example/profile/files", UnitOfWork: passthrough{}, Clock: clockAt{now}, IDs: &counter{},
	}
	return f
}

func actorOf(account shared.ID) appshared.ActorContext {
	return appshared.ActorContext{Kind: appshared.ActorUser, TenantID: tenant, AccountID: account, Scopes: []string{readScope, writeScope}}
}

func call(d usecase.Descriptor, actor appshared.ActorContext, in map[string]any) (usecase.Output, error) {
	return d.Handler.Invoke(context.Background(), actor, usecase.Input(in))
}

func codeOf(err error) string {
	var e *shared.Error
	if errors.As(err, &e) {
		return e.DetailCode
	}
	return ""
}

func connectNextcloud(t *testing.T, f fixture) shared.ID {
	t.Helper()
	started, err := call(ConnectFileStore{f.conns}.Descriptor(), actorOf(owner), map[string]any{"kind": "NEXTCLOUD", "server": "cloud.example.org"})
	if err != nil {
		t.Fatal(err)
	}
	if started["authorize_url"] != "https://cloud.example.org/login/v2/flow/x" || started["state"] == "" {
		t.Fatalf("started = %v", started)
	}
	f.files.granted = true
	done, err := call(CompleteFileStoreConnection{f.conns}.Descriptor(), actorOf(owner), map[string]any{"state": started["state"]})
	if err != nil {
		t.Fatal(err)
	}
	return shared.ID(done["id"].(string))
}

// UC-INT-17 check 1: a Nextcloud is connected by its login flow - not granted yet keeps it
// waiting - and its app password is kept sealed; nobody else finishes the owner's connection.
func TestANextcloudIsConnectedByItsLoginFlow(t *testing.T) {
	f := newFixture()
	started, err := call(ConnectFileStore{f.conns}.Descriptor(), actorOf(owner), map[string]any{"kind": "NEXTCLOUD", "server": "https://cloud.example.org/index.php/apps/files"})
	if err != nil {
		t.Fatal(err)
	}
	complete := CompleteFileStoreConnection{f.conns}.Descriptor()
	if _, err := call(complete, actorOf(owner), map[string]any{"state": started["state"]}); codeOf(err) != "filestore.not_granted_yet" {
		t.Fatalf("not yet granted answered %v", err)
	}
	if _, err := call(complete, actorOf(stranger), map[string]any{"state": started["state"]}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("a stranger finished the connection: %v", err)
	}
	f.files.granted = true
	done, err := call(complete, actorOf(owner), map[string]any{"state": started["state"]})
	if err != nil || done["server"] != "https://cloud.example.org" || done["login"] != "ada" {
		t.Fatalf("done = %v %v", done, err)
	}
	if len(f.store.pending) != 0 {
		t.Fatal("the waiting connection was kept")
	}
	sealed := f.store.sealed[shared.ID(done["id"].(string))]
	if string(sealed.Ciphertext) != "app-pw" || !strings.HasPrefix(sealed.KeyID, "filestore.secret:") {
		t.Fatalf("sealed = %+v", sealed)
	}
	if _, err := call(complete, actorOf(owner), map[string]any{"state": started["state"]}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("a state was used twice: %v", err)
	}
}

// UC-INT-17 check 1: Microsoft is connected by the installation's app with PKCE and a consent to
// read files alone, and is not offered without the app.
func TestMicrosoftIsConnectedWithAConsentToReadFiles(t *testing.T) {
	f := newFixture()
	started, err := call(ConnectFileStore{f.conns}.Descriptor(), actorOf(owner), map[string]any{"kind": "MICROSOFT"})
	if err != nil {
		t.Fatal(err)
	}
	address := started["authorize_url"].(string)
	if !strings.Contains(address, "Files.Read.All") || !strings.Contains(address, "code_challenge=") || strings.Contains(address, "Mail") {
		t.Fatalf("authorize_url = %s", address)
	}
	state := address[strings.Index(address, "state=")+6:]
	if i := strings.Index(state, "&"); i >= 0 {
		state = state[:i]
	}
	done, err := call(CompleteFileStoreConnection{f.conns}.Descriptor(), actorOf(owner), map[string]any{"state": state, "code": "c1"})
	if err != nil || done["login"] != "ada@example.org" || done["kind"] != "MICROSOFT" {
		t.Fatalf("done = %v %v", done, err)
	}
	listed, _ := call(ListFileStoreConnections{f.conns}.Descriptor(), actorOf(owner), nil)
	if kinds := listed["kinds"].([]string); len(kinds) != 2 {
		t.Fatalf("kinds = %v", kinds)
	}
	f.conns.Apps = nil
	if _, err := call(ConnectFileStore{f.conns}.Descriptor(), actorOf(owner), map[string]any{"kind": "MICROSOFT"}); codeOf(err) != "filestore.kind_not_offered" {
		t.Fatalf("without an app: %v", err)
	}
}

// UC-INT-17 checks 2 and 4: the owner browses their own store - Microsoft's with an access token
// drawn now -, and nobody else lists, browses, shares with or disconnects the connection.
func TestOnlyTheOwnerBrowsesTheirConnection(t *testing.T) {
	f := newFixture()
	id := connectNextcloud(t, f)
	browse := BrowseFileStore{f.conns}.Descriptor()
	out, err := call(browse, actorOf(owner), map[string]any{"connection_id": id.String(), "folder": "/Specs"})
	if err != nil || len(out["data"].([]usecase.Output)) != 1 || f.files.access[0].Secret.Reveal() != "app-pw" {
		t.Fatalf("browse = %v %v", out, err)
	}
	if _, err := call(browse, actorOf(owner), map[string]any{"connection_id": id.String(), "folder": "/../bob"}); codeOf(err) != "filestore.folder_invalid" {
		t.Fatalf("a step out: %v", err)
	}
	for _, d := range []usecase.Descriptor{browse, ShareFileStoreItem{f.conns}.Descriptor(), DisconnectFileStore{f.conns}.Descriptor()} {
		if _, err := call(d, actorOf(stranger), map[string]any{"connection_id": id.String(), "ref": "/Spec.pdf"}); !errors.Is(err, shared.ErrNotFound) {
			t.Fatalf("%s: a stranger reached the connection: %v", d.Name, err)
		}
	}
	if listed, _ := call(ListFileStoreConnections{f.conns}.Descriptor(), actorOf(stranger), nil); len(listed["data"].([]usecase.Output)) != 0 {
		t.Fatal("a stranger listed the connection")
	}
	f.store.conns[msID] = filelink.Connection{ID: msID, AccountID: owner, Kind: filelink.ConnectMicrosoft, Login: "ada@example.org"}
	f.store.sealed[msID] = crypto.Sealed{KeyID: string(secretPurpose(msID)), Ciphertext: []byte("refresh")}
	if _, err := call(browse, actorOf(owner), map[string]any{"connection_id": msID.String()}); err != nil {
		t.Fatal(err)
	}
	if got := f.files.access[len(f.files.access)-1].Secret.Reveal(); got != "access-of-refresh" {
		t.Fatalf("Microsoft was browsed with %q", got)
	}
}

// UC-INT-17 check 3: a share link is a Nextcloud's alone, and a disconnect gives its app
// password back.
func TestAShareIsANextcloudsAndADisconnectGivesThePasswordBack(t *testing.T) {
	f := newFixture()
	id := connectNextcloud(t, f)
	out, err := call(ShareFileStoreItem{f.conns}.Descriptor(), actorOf(owner), map[string]any{"connection_id": id.String(), "ref": "/Spec.pdf"})
	if err != nil || out["address"] != "https://cloud.example.org/s/abc" {
		t.Fatalf("share = %v %v", out, err)
	}
	f.store.conns[msID] = filelink.Connection{ID: msID, AccountID: owner, Kind: filelink.ConnectMicrosoft}
	f.store.sealed[msID] = crypto.Sealed{KeyID: string(secretPurpose(msID)), Ciphertext: []byte("refresh")}
	if _, err := call(ShareFileStoreItem{f.conns}.Descriptor(), actorOf(owner), map[string]any{"connection_id": msID.String(), "ref": "x"}); codeOf(err) != "filestore.share_not_offered" {
		t.Fatalf("a Microsoft share: %v", err)
	}
	if _, err := call(DisconnectFileStore{f.conns}.Descriptor(), actorOf(owner), map[string]any{"connection_id": id.String()}); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.store.conns[id]; ok || len(f.files.revoked) != 1 || f.files.revoked[0] != "app-pw" {
		t.Fatalf("disconnect left %v, revoked %v", f.store.conns, f.files.revoked)
	}
}

func TestAPersonHoldsAtMostTenConnections(t *testing.T) {
	f := newFixture()
	for i := 0; i < filelink.MaxConnections; i++ {
		id := shared.ID(fmt.Sprintf("c%d", i))
		f.store.conns[id] = filelink.Connection{ID: id, AccountID: owner}
	}
	if _, err := call(ConnectFileStore{f.conns}.Descriptor(), actorOf(owner), map[string]any{"kind": "NEXTCLOUD", "server": "cloud.example.org"}); codeOf(err) != "filestore.connections_full" {
		t.Fatalf("eleventh: %v", err)
	}
}
