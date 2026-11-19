// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package media

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/media"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/wopi"
)

// chosenServer is a workspace that chose Collabora Online; the rest of the store is not asked.
type chosenServer struct{ repository.Wopi }

func (chosenServer) DocumentServer(context.Context) (domain.DocumentServerChoice, error) {
	return domain.NewDocumentServerChoice(domain.DocumentServerCollabora, "https://office.example")
}

// plainTokens carries the grant itself, so a test can read back what was issued.
type plainTokens struct{ issued map[string]wopi.Grant }

func (p *plainTokens) Issue(grant wopi.Grant) string {
	if p.issued == nil {
		p.issued = map[string]wopi.Grant{}
	}
	token := grant.AccountID.String() + "/" + grant.MediaID.String()
	p.issued[token] = grant
	return token
}

func (p *plainTokens) Read(token string, mediaID shared.ID, at time.Time) (wopi.Grant, error) {
	grant, ok := p.issued[token]
	if !ok || grant.MediaID != mediaID || at.After(grant.ExpiresAt) {
		return wopi.Grant{}, wopi.ErrInvalidToken()
	}
	return grant, nil
}

// pageWriter answers as the knowledge module would, and remembers who asked about which pages.
type pageWriter struct {
	pageReader
	writes  bool
	written [][]shared.ID
	scopes  [][]string
}

func (p *pageWriter) WritesAny(_ context.Context, actor appshared.ActorContext, pages []shared.ID) (bool, error) {
	p.written = append(p.written, pages)
	p.scopes = append(p.scopes, actor.Scopes)
	return p.writes, nil
}

func pageHeldDocument(writes bool) (WopiHost, *pageWriter, *plainTokens, shared.ID) {
	page := shared.MustParseID("01936f2a-7c1e-7000-8000-00000000d311")
	read, records, _, _ := readingHarness()
	pages := &pageWriter{pageReader: pageReader{reads: true}, writes: writes}
	read.Pages = pages
	object := ready(strangerA)
	object.FileName = "plan.docx"
	object.ContentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	records.stored[mintedID] = object
	records.pagesHolding = map[shared.ID][]shared.ID{mintedID: {page}}
	tokens := &plainTokens{}
	return WopiHost{
		Read: read, Wopi: chosenServer{}, Tokens: tokens, Pages: pages, Server: &probe{},
		UnitOfWork: read.UnitOfWork, Clock: read.Clock, Files: "https://hub.example/api/v1/wopi/files/",
	}, pages, tokens, page
}

// UC-MED-06: a page's office file is edited in the document server by whoever may write the
// page - a question the knowledge module answers, asked with the page that holds the file - and
// by nobody else; the reader of the page still reads it.
func TestAPagesFileIsEditedByWhoeverMayWriteThePage(t *testing.T) {
	for _, writes := range []bool{true, false} {
		host, pages, _, page := pageHeldDocument(writes)
		session, err := host.StartEditing(t.Context(), actor(mediaRead, mediaWrite), mintedID)
		if writes && (err != nil || session.Token == "") {
			t.Fatalf("a writer of the page was refused: %v", err)
		}
		// The browser posts the token to the server's editor, the WOPI source in its query.
		if want := "https://office.example/browser/abc/cool.html?WOPISrc=https%3A%2F%2Fhub.example%2Fapi%2Fv1%2Fwopi%2Ffiles%2F" + mintedID.String(); writes && session.ActionURL != want {
			t.Errorf("action %q, want %q", session.ActionURL, want)
		}
		if !writes && !errors.Is(err, shared.ErrForbidden) {
			t.Fatalf("somebody who may not write the page got %v, want forbidden", err)
		}
		if len(pages.written) != 1 || len(pages.written[0]) != 1 || pages.written[0][0] != page {
			t.Fatalf("asked %v, want the page", pages.written)
		}
	}
}

// UC-MED-06: the document server's calls carry the person's token, and the page is asked again
// with their own question - their page rights, never more than the token's write flag.
func TestTheDocumentServersCallsAskThePageAgain(t *testing.T) {
	host, pages, tokens, _ := pageHeldDocument(true)
	session, err := host.StartEditing(t.Context(), actor(mediaRead, mediaWrite), mintedID)
	if err != nil {
		t.Fatal(err)
	}
	info, err := host.CheckFileInfo(t.Context(), session.Token, mintedID)
	if err != nil || !info.Write {
		t.Fatalf("CheckFileInfo %+v, %v; want a writer", info, err)
	}
	if last := pages.scopes[len(pages.scopes)-1]; !hasAll(last, "items:read", "items:write") {
		t.Fatalf("the page was asked with scopes %v", last)
	}

	pages.writes = false
	if info, err := host.CheckFileInfo(t.Context(), session.Token, mintedID); err != nil || info.Write {
		t.Fatalf("a person who lost the page's write right may still save: %+v, %v", info, err)
	}

	// A read token asks the page as a reader only.
	read := tokens.Issue(wopi.Grant{TenantID: tenantID, AccountID: accountID, MediaID: mintedID, ExpiresAt: now.Add(time.Hour)})
	pages.writes = true
	if info, err := host.CheckFileInfo(t.Context(), read, mintedID); err != nil || info.Write {
		t.Fatalf("a read token may save: %+v, %v", info, err)
	}
}

func hasAll(scopes []string, want ...string) bool {
	for _, w := range want {
		found := false
		for _, s := range scopes {
			found = found || s == w
		}
		if !found {
			return false
		}
	}
	return true
}

// probe is a document server that answers its discovery as told, and remembers what it was asked.
type probe struct {
	answer error
	asked  []domain.DocumentServerChoice
	// pdf is what a conversion answers, none converting nothing; converted is what it was sent.
	pdf       string
	converted []wopi.Source
}

func (p *probe) ConvertToPDF(_ context.Context, _ domain.DocumentServerChoice, source wopi.Source) (io.ReadCloser, error) {
	if p.pdf == "" {
		return nil, wopi.ErrUnsupported()
	}
	p.converted = append(p.converted, source)
	return io.NopCloser(strings.NewReader(p.pdf)), nil
}

func (p *probe) EditAction(_ context.Context, choice domain.DocumentServerChoice, _ string) (string, error) {
	return choice.URL + "/browser/abc/cool.html?", p.answer
}

func (p *probe) Reach(_ context.Context, choice domain.DocumentServerChoice) error {
	p.asked = append(p.asked, choice)
	return p.answer
}

// UC-MED-06 check 1: the administration tests a server before choosing it - under STRUCTURE at
// the workspace, never an agent, never an address the choice itself refuses - and the server's
// refusal reaches it as the server's own code.
func TestTheAdministrationTestsADocumentServer(t *testing.T) {
	servers := func(server *probe, permission *authorizer) DocumentServers {
		return DocumentServers{Authorizer: permission, Server: server, UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now)}
	}
	admin := actor(mediaRead, mediaWrite)

	server, permission := &probe{}, &authorizer{}
	if err := servers(server, permission).Check(t.Context(), admin, "COLLABORA_ONLINE", "https://office.example"); err != nil {
		t.Fatalf("a reachable server: %v", err)
	}
	if len(server.asked) != 1 || server.asked[0].URL != "https://office.example" {
		t.Fatalf("asked %+v", server.asked)
	}
	if asked := permission.requests; len(asked) != 1 || asked[0].Permission != service.PermissionStructure ||
		len(asked[0].Path) != 1 || asked[0].Path[0] != identity.TenantScope() {
		t.Fatalf("asked %+v, want STRUCTURE at the workspace", asked)
	}

	private := shared.ErrValidation.WithDetail("media.document_server_private_network")
	server = &probe{answer: private}
	if err := servers(server, &authorizer{}).Check(t.Context(), admin, "ONLYOFFICE", "https://office.internal"); !errors.Is(err, private) {
		t.Fatalf("the server's refusal: got %v", err)
	}

	agent := admin
	agent.Kind = appshared.ActorAIAgent
	for _, c := range []struct {
		name       string
		actor      appshared.ActorContext
		permission *authorizer
		kind, url  string
	}{
		{"an agent", agent, &authorizer{}, "COLLABORA_ONLINE", "https://office.example"},
		{"no STRUCTURE", admin, &authorizer{err: shared.ErrForbidden}, "COLLABORA_ONLINE", "https://office.example"},
		{"a plain http address", admin, &authorizer{}, "COLLABORA_ONLINE", "http://office.example"},
		{"no server chosen", admin, &authorizer{}, "NONE", ""},
	} {
		server := &probe{}
		if err := servers(server, c.permission).Check(t.Context(), c.actor, c.kind, c.url); err == nil {
			t.Errorf("%s: tested", c.name)
		}
		if len(server.asked) != 0 {
			t.Errorf("%s: the server was asked", c.name)
		}
	}
}

// files is the WOPI store: the workspace's choice, the lock on a file and its saves; a save gives
// the file its new bytes, as the repository does.
type files struct {
	repository.Wopi
	choice  domain.DocumentServerChoice
	setBy   shared.ID
	lock    *domain.Lock
	dropped int
	saves   []repository.Save
	records *objects
	saveErr error
}

func (f *files) DocumentServer(context.Context) (domain.DocumentServerChoice, error) {
	return f.choice, nil
}

func (f *files) SetDocumentServer(_ context.Context, choice domain.DocumentServerChoice, setBy shared.ID, _ time.Time) error {
	f.choice, f.setBy = choice, setBy
	return nil
}

func (f *files) Lock(context.Context, shared.ID) (*domain.Lock, error) { return f.lock, nil }

func (f *files) PutLock(_ context.Context, _ shared.ID, lock domain.Lock) error {
	f.lock = &lock
	return nil
}

func (f *files) DropLock(context.Context, shared.ID) error {
	f.lock = nil
	f.dropped++
	return nil
}

func (f *files) SaveVersion(_ context.Context, mediaID shared.ID, save repository.Save) (int, error) {
	if f.saveErr != nil {
		return 0, f.saveErr
	}
	f.saves = append(f.saves, save)
	object := f.records.stored[mediaID]
	object.StorageKey, object.ContentType, object.ByteSize = save.StorageKey, save.ContentType, save.ByteSize
	f.records.stored[mediaID] = object
	return len(f.saves), nil
}

// editingHarness is a page's office file a writer of the page edits in Collabora, its bytes in
// the store, and a write token for them.
type editingHarness struct {
	host   WopiHost
	pages  *pageWriter
	tokens *plainTokens
	files  *files
	bytes  *store
	server *probe
	token  string
}

func newEditingHarness(t *testing.T) *editingHarness {
	t.Helper()
	host, pages, tokens, _ := pageHeldDocument(true)
	records := host.Read.Objects.(*objects)
	choice, err := domain.NewDocumentServerChoice(domain.DocumentServerCollabora, "https://office.example")
	if err != nil {
		t.Fatal(err)
	}
	h := &editingHarness{
		pages: pages, tokens: tokens, files: &files{choice: choice, records: records}, bytes: newStore(),
		server: &probe{pdf: "%PDF-1.7"},
	}
	object := records.stored[mintedID]
	h.bytes.content[object.StorageKey] = []byte("the offer")
	host.Wopi, host.Store, host.Server = h.files, h.bytes, h.server
	host.Guard = &guard{judged: object.ContentType}
	host.IDs = &ids{next: shared.MustParseID("0192f000-0000-7000-8000-0000000000e1")}
	h.host = host
	session, err := host.StartEditing(t.Context(), actor(mediaRead, mediaWrite), mintedID)
	if err != nil {
		t.Fatal(err)
	}
	h.token = session.Token
	return h
}

// readToken is a token that only reads the file.
func (h *editingHarness) readToken() string {
	return h.tokens.Issue(wopi.Grant{TenantID: tenantID, AccountID: strangerA, MediaID: mintedID, ExpiresAt: now.Add(time.Hour)})
}

// UC-MED-06 check 5: a lock is taken on an unlocked file, renewed and replaced only by the one
// that holds it, and released by it alone; another lock is refused with the lock that holds.
func TestTheDocumentServersLockHoldsAgainstAnother(t *testing.T) {
	h := newEditingHarness(t)
	steps := []struct {
		name     string
		request  LockRequest
		answer   string
		conflict *string
	}{
		{"nothing held", LockRequest{Op: "GET_LOCK"}, "", nil},
		{"taken", LockRequest{Op: "LOCK", Lock: "A"}, "A", nil},
		{"another is refused", LockRequest{Op: "LOCK", Lock: "B"}, "", ptr("A")},
		{"renewed", LockRequest{Op: "REFRESH_LOCK", Lock: "A"}, "A", nil},
		{"replaced by its holder", LockRequest{Op: "UNLOCK_AND_RELOCK", Old: "A", Lock: "B"}, "B", nil},
		{"replaced by another", LockRequest{Op: "UNLOCK_AND_RELOCK", Old: "A", Lock: "C"}, "", ptr("B")},
		{"read", LockRequest{Op: "GET_LOCK"}, "B", nil},
		{"released by another", LockRequest{Op: "UNLOCK", Lock: "A"}, "", ptr("B")},
		{"released", LockRequest{Op: "UNLOCK", Lock: "B"}, "", nil},
		{"renewed once released", LockRequest{Op: "REFRESH_LOCK", Lock: "B"}, "", ptr("")},
	}
	for _, step := range steps {
		answer, err := h.host.Locks(t.Context(), h.token, mintedID, step.request)
		var conflict domain.LockConflict
		switch {
		case step.conflict == nil && (err != nil || answer != step.answer):
			t.Fatalf("%s: %q, %v; want %q", step.name, answer, err, step.answer)
		case step.conflict != nil && (!errors.As(err, &conflict) || conflict.Current != *step.conflict):
			t.Fatalf("%s: %v, want the conflict naming %q", step.name, err, *step.conflict)
		}
	}
	if h.files.lock != nil || h.files.dropped != 1 {
		t.Fatalf("lock %+v after the release, dropped %d", h.files.lock, h.files.dropped)
	}
	if _, err := h.host.Locks(t.Context(), h.token, mintedID, LockRequest{Op: "STEAL"}); detail(err) != "wopi.operation_unknown" {
		t.Fatalf("an unknown call: %v", err)
	}

	// A read token reads the lock and takes none.
	read := h.readToken()
	if _, err := h.host.Locks(t.Context(), read, mintedID, LockRequest{Op: "GET_LOCK"}); err != nil {
		t.Fatalf("reading the lock with a read token: %v", err)
	}
	if _, err := h.host.Locks(t.Context(), read, mintedID, LockRequest{Op: "LOCK", Lock: "R"}); detail(err) != "media.edit_forbidden" {
		t.Fatalf("locking with a read token: %v", err)
	}
	// Whoever lost the right to change the page takes no lock either (check 4).
	h.pages.writes = false
	if _, err := h.host.Locks(t.Context(), h.token, mintedID, LockRequest{Op: "LOCK", Lock: "A"}); detail(err) != "media.edit_forbidden" {
		t.Fatalf("locking without the page's write right: %v", err)
	}
	if h.files.lock != nil {
		t.Fatal("a refused call took a lock")
	}
}

func ptr(s string) *string { return &s }

// UC-MED-06 checks 5 and 6: a save under the lock that holds is a new version - the file keeps its
// id, takes new bytes under a new key, and the bytes it had are kept as its earlier version; under
// another lock, too large, or by a read token nothing is stored.
func TestASaveUnderTheLockIsANewVersionKeepingTheEarlier(t *testing.T) {
	h := newEditingHarness(t)
	before := h.host.Read.Objects.(*objects).stored[mintedID]
	if _, err := h.host.Locks(t.Context(), h.token, mintedID, LockRequest{Op: "LOCK", Lock: "A"}); err != nil {
		t.Fatal(err)
	}
	info, err := h.host.CheckFileInfo(t.Context(), h.token, mintedID)
	if err != nil || !info.Write || info.PersonID != accountID {
		t.Fatalf("CheckFileInfo %+v, %v", info, err)
	}

	refusals := []struct {
		name  string
		token string
		lock  string
		size  int64
		check func(error) bool
	}{
		{"another lock", h.token, "B", 9, func(err error) bool {
			var conflict domain.LockConflict
			return errors.As(err, &conflict) && conflict.Current == "A"
		}},
		{"too large", h.token, "A", 1 << 20, func(err error) bool { return detail(err) == "media.too_large" }},
		{"a negative size", h.token, "A", -1, func(err error) bool { return detail(err) == "media.too_large" }},
		{"a read token", h.readToken(), "A", 9, func(err error) bool { return detail(err) == "media.edit_forbidden" }},
		{"no token", "", "A", 9, func(err error) bool { return detail(err) == "wopi.token_invalid" }},
	}
	h.host.SizeLimit = 1024
	for _, refusal := range refusals {
		if _, err := h.host.PutFile(t.Context(), refusal.token, mintedID, refusal.lock, strings.NewReader("new offer"), refusal.size); !refusal.check(err) {
			t.Fatalf("%s: %v", refusal.name, err)
		}
	}
	if len(h.bytes.puts) != 0 || len(h.files.saves) != 0 {
		t.Fatal("a refused save stored something")
	}

	version, err := h.host.PutFile(t.Context(), h.token, mintedID, "A", strings.NewReader("new offer"), 9)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.files.saves) != 1 || len(h.bytes.puts) != 1 {
		t.Fatalf("%d saves, %d puts", len(h.files.saves), len(h.bytes.puts))
	}
	save := h.files.saves[0]
	if save.StorageKey == before.StorageKey || string(h.bytes.content[save.StorageKey]) != "new offer" || save.ByteSize != 9 {
		t.Fatalf("save %+v, want the new bytes under a new key", save)
	}
	if save.Earlier.StorageKey != before.StorageKey || save.Earlier.ID == mintedID || save.Earlier.Usage != domain.UsageAttachment ||
		string(h.bytes.content[before.StorageKey]) != "the offer" {
		t.Fatalf("earlier %+v, want the bytes it had kept as an object of their own", save.Earlier)
	}
	if save.SavedBy != accountID || !save.SavedAt.Equal(now) {
		t.Errorf("saved by %s at %v", save.SavedBy, save.SavedAt)
	}
	after, err := h.host.CheckFileInfo(t.Context(), h.token, mintedID)
	if err != nil || after.Object.ID != mintedID || after.Version != version || version == info.Version {
		t.Fatalf("after the save %+v (%v), version %q, before %q", after, err, version, info.Version)
	}
}

// UC-MED-06 check 5: an unlocked file that is still empty takes a save without a lock; one with
// bytes does not.
func TestAnEmptyFileTakesASaveWithoutALock(t *testing.T) {
	for _, size := range []int64{0, 32} {
		h := newEditingHarness(t)
		records := h.host.Read.Objects.(*objects)
		object := records.stored[mintedID]
		object.ByteSize = size
		records.stored[mintedID] = object
		_, err := h.host.PutFile(t.Context(), h.token, mintedID, "", strings.NewReader("first"), 5)
		if size == 0 && err != nil {
			t.Fatalf("saving into an empty file: %v", err)
		}
		var conflict domain.LockConflict
		if size > 0 && !errors.As(err, &conflict) {
			t.Fatalf("saving into a file with bytes without a lock: %v", err)
		}
	}
}

// A save the store refuses leaves no bytes behind: the new object is removed at once.
func TestAFailedSaveRemovesItsBytes(t *testing.T) {
	h := newEditingHarness(t)
	if _, err := h.host.Locks(t.Context(), h.token, mintedID, LockRequest{Op: "LOCK", Lock: "A"}); err != nil {
		t.Fatal(err)
	}
	h.files.saveErr = shared.ErrNotFound
	_, err := h.host.PutFile(t.Context(), h.token, mintedID, "A", strings.NewReader("new offer"), 9)
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("err %v, want the file missing", err)
	}
	if len(h.bytes.puts) != 1 || len(h.bytes.deleted) != 1 || h.bytes.deleted[0] != h.bytes.puts[0].Key {
		t.Fatalf("put %+v, deleted %v; want the new bytes removed", h.bytes.puts, h.bytes.deleted)
	}
}

// UC-MED-06 check 3: GetFile answers the file's bytes for its token alone; a forged token, or one
// for another file, opens nothing.
func TestTheFileIsServedForItsTokenAlone(t *testing.T) {
	h := newEditingHarness(t)
	served, err := h.host.GetFile(t.Context(), h.token, mintedID)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(served.Content)
	_ = served.Content.Close()
	if string(body) != "the offer" {
		t.Fatalf("served %q", body)
	}
	other := shared.MustParseID("0192f000-0000-7000-8000-0000000000e9")
	for name, call := range map[string]func() error{
		"forged":       func() error { _, err := h.host.GetFile(t.Context(), "forged", mintedID); return err },
		"another file": func() error { _, err := h.host.GetFile(t.Context(), h.token, other); return err },
		"empty":        func() error { _, err := h.host.CheckFileInfo(t.Context(), "", mintedID); return err },
	} {
		if err := call(); detail(err) != "wopi.token_invalid" {
			t.Errorf("%s token: %v", name, err)
		}
	}
}

// UC-MED-05: an office file is shown as the PDF the document server converts; the server reaches
// the bytes with a read token as short as the conversion, never a write token.
func TestAnOfficeFileIsShownAsTheServersPDF(t *testing.T) {
	h := newEditingHarness(t)
	pdf, err := h.host.ConvertToPDF(t.Context(), actor(mediaRead), mintedID)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(pdf)
	_ = pdf.Close()
	if string(body) != "%PDF-1.7" || len(h.server.converted) != 1 {
		t.Fatalf("converted %q, %d sources", body, len(h.server.converted))
	}
	source := h.server.converted[0]
	prefix := h.host.Files + mintedID.String() + "/contents?access_token="
	if !strings.HasPrefix(source.FetchAt, prefix) || source.Name != "plan.docx" || !strings.HasPrefix(source.Key, mintedID.String()+"-") {
		t.Fatalf("source %+v", source)
	}
	grant := h.tokens.issued[strings.TrimPrefix(source.FetchAt, prefix)]
	if grant.Write || grant.MediaID != mintedID || !grant.ExpiresAt.Equal(now.Add(conversionWindow)) {
		t.Fatalf("the conversion's token %+v, want a short read token", grant)
	}

	if _, err := h.host.ConvertToPDF(t.Context(), actor(), mintedID); err == nil {
		t.Error("converted without the read scope")
	}
	records := h.host.Read.Objects.(*objects)
	picture := records.stored[mintedID]
	picture.FileName, picture.ContentType = "plan.png", "image/png"
	records.stored[mintedID] = picture
	if _, err := h.host.ConvertToPDF(t.Context(), actor(mediaRead), mintedID); detail(err) != "media.not_an_office_file" {
		t.Errorf("a picture: %v", err)
	}
	if _, err := h.host.StartEditing(t.Context(), actor(mediaRead, mediaWrite), mintedID); detail(err) != "media.not_an_office_file" {
		t.Errorf("editing a picture: %v", err)
	}
}

// UC-MED-06 check 2: *Bearbeiten* is offered only on a ready office file, only where a server is
// chosen, and only to whoever may change what holds it.
func TestEditingIsOfferedOnlyWhereItCanHappen(t *testing.T) {
	tests := []struct {
		name    string
		arrange func(*editingHarness, *domain.Object)
		want    bool
	}{
		{"a writer of the page", func(*editingHarness, *domain.Object) {}, true},
		{"a reader of the page", func(h *editingHarness, _ *domain.Object) { h.pages.writes = false }, false},
		{"no server chosen", func(h *editingHarness, _ *domain.Object) { h.files.choice = domain.DocumentServerChoice{} }, false},
		{"a picture", func(_ *editingHarness, o *domain.Object) { o.FileName, o.ContentType = "plan.png", "image/png" }, false},
		{"a file not yet judged", func(_ *editingHarness, o *domain.Object) { o.Status = domain.StatusPending }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newEditingHarness(t)
			object := h.host.Read.Objects.(*objects).stored[mintedID]
			tt.arrange(h, &object)
			got, err := h.host.Editable(t.Context(), actor(mediaRead, mediaWrite), object, reach{pages: []shared.ID{h.pages.written[0][0]}})
			if err != nil || got != tt.want {
				t.Fatalf("editable %v, %v; want %v", got, err, tt.want)
			}
		})
	}

	h := newEditingHarness(t)
	h.files.choice = domain.DocumentServerChoice{}
	if _, err := h.host.StartEditing(t.Context(), actor(mediaRead, mediaWrite), mintedID); detail(err) != "media.document_server_none" {
		t.Errorf("editing without a server: %v", err)
	}
	if _, err := h.host.StartEditing(t.Context(), actor(mediaRead), mintedID); err == nil {
		t.Error("editing without the write scope")
	}
}

// UC-MED-06 check 1: every member reads the choice; the administration sets it, audited with the
// kind and the address, and an agent never does.
func TestTheWorkspaceChoosesItsDocumentServer(t *testing.T) {
	store, permission, trail := &files{}, &authorizer{}, &sink{}
	servers := DocumentServers{Wopi: store, Authorizer: permission, Audit: trail, UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now)}
	admin := actor(mediaRead, mediaWrite)

	read := ReadDocumentServer{Servers: servers}.Descriptor().Handler
	if out, err := read.Invoke(t.Context(), actor(mediaRead), usecase.Input{}); err != nil || out["kind"] != "NONE" || out["url"] != nil {
		t.Fatalf("nothing chosen reads %+v, %v", out, err)
	}
	if _, err := read.Invoke(t.Context(), actor(), usecase.Input{}); err == nil {
		t.Fatal("read without the read scope")
	}

	set := SetDocumentServer{Servers: servers}.Descriptor().Handler
	out, err := set.Invoke(t.Context(), admin, usecase.Input{"kind": "ONLYOFFICE", "url": "https://office.example"})
	if err != nil || out["kind"] != "ONLYOFFICE" || out["url"] != "https://office.example" {
		t.Fatalf("set %+v, %v", out, err)
	}
	if store.choice.Kind != domain.DocumentServerOnlyOffice || store.setBy != accountID {
		t.Fatalf("stored %+v by %s", store.choice, store.setBy)
	}
	if len(trail.entries) != 1 || trail.entries[0].Action != DocumentServerSetAction {
		t.Fatalf("trail %+v", trail.entries)
	}
	kind, _ := trail.entries[0].Changes["kind"].(map[string]any)
	if kind["to"] != "ONLYOFFICE" {
		t.Errorf("audited %+v", trail.entries[0].Changes)
	}
	if out, err := read.Invoke(t.Context(), actor(mediaRead), usecase.Input{}); err != nil || out["url"] != "https://office.example" {
		t.Fatalf("the choice reads %+v, %v", out, err)
	}

	agent := admin
	agent.Kind = appshared.ActorAIAgent
	for name, attempt := range map[string]struct {
		actor appshared.ActorContext
		kind  string
	}{"an agent": {agent, "COLLABORA_ONLINE"}, "an unknown kind": {admin, "TYPEWRITER"}} {
		if _, err := set.Invoke(t.Context(), attempt.actor, usecase.Input{"kind": attempt.kind, "url": "https://other.example"}); err == nil {
			t.Errorf("%s set the server", name)
		}
	}
	permission.err = shared.ErrForbidden
	if _, err := set.Invoke(t.Context(), admin, usecase.Input{"kind": "NONE"}); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("without STRUCTURE: %v", err)
	}
	if store.choice.Kind != domain.DocumentServerOnlyOffice || len(trail.entries) != 1 {
		t.Fatal("a refused choice changed something")
	}

	permission.err = nil
	check := CheckDocumentServer{Servers: DocumentServers{Authorizer: permission, Server: &probe{}, UnitOfWork: &unitOfWork{}}}.Descriptor().Handler
	if out, err := check.Invoke(t.Context(), admin, usecase.Input{"kind": "ONLYOFFICE", "url": "https://office.example"}); err != nil || out["reached"] != true {
		t.Fatalf("check %+v, %v", out, err)
	}
}

func detail(err error) string {
	var coded *shared.Error
	if errors.As(err, &coded) {
		return coded.DetailCode
	}
	return ""
}
