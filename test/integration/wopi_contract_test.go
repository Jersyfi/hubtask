// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	mediarepo "github.com/Jersyfi/hubtask/core/application/repository/media"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	mediaservice "github.com/Jersyfi/hubtask/core/application/service/media"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
	storageport "github.com/Jersyfi/hubtask/core/port/storage"
	"github.com/Jersyfi/hubtask/core/port/wopi"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
	"github.com/Jersyfi/hubtask/infrastructure/storage"
	"github.com/Jersyfi/hubtask/presentation/rest"
)

// The WOPI host's contract, driven the way a document server drives it (UC-MED-06): a stub
// server opens the file with the person's token, reads its information and bytes, locks it,
// saves under the lock - which is a new version - and unlocks. Beside it what the protocol
// refuses: a save under another lock, a reader's save, a forged token, a token of another file.

// writesOnly lets everyone read and only the writer change entries (rule 2's question, answered).
type writesOnly struct{ writer shared.ID }

func (w writesOnly) Permitted(
	_ context.Context, actor appshared.ActorContext, request access.Request, paths [][]identity.Scope,
) ([]bool, error) {
	out := make([]bool, len(paths))
	for i := range out {
		out[i] = request.Permission == service.PermissionRead || actor.AccountID == w.writer
	}
	return out, nil
}

// answered is one response, read whole and closed.
type answered struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

var docx = append([]byte("PK\x03\x04"), bytes.Repeat([]byte{1}, 60)...)

func TestADocumentServerEditsAFileThroughTheWopiHost(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	task := seedTask(ctx, t, tenantA, authorA, collection)

	store := storage.NewLocalStorage(t.TempDir())
	object, err := media.NewPendingObject(media.NewObjectInput{
		ID: freshID(t), TenantID: tenantA, FileName: "plan.docx",
		ClaimedType:  "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		DeclaredSize: int64(len(docx)), SizeLimit: 1 << 20, Usage: media.UsageAttachment,
		CreatedBy: authorA, Now: created,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, storageport.Upload{Key: object.StorageKey, Content: bytes.NewReader(docx), Size: int64(len(docx)), ContentType: "application/zip"}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error { return mediaRepo().Insert(ctx, object) }); err != nil {
		t.Fatal(err)
	}
	sealed, _ := object.Sealed(object.ContentType, int64(len(docx)))
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := mediaRepo().Seal(ctx, sealed); err != nil {
			return err
		}
		_, err := mediaRepo().Add(ctx, task, object.ID, shared.HLC{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	choice, _ := media.NewDocumentServerChoice(media.DocumentServerCollabora, "https://office.example")
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return postgres.WopiRepository{}.SetDocumentServer(ctx, choice, authorA, created)
	}); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	tokens := security.NewWopiTokenIssuer(secret.New("an installation secret of decent length"))
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	host := mediaservice.WopiHost{
		Read: mediaservice.GetMedia{
			Objects: mediaRepo(), Containers: containerRepo(), Reader: writesOnly{writer: authorA},
			UnitOfWork: uow, Clock: clock.Fixed(now),
		},
		Wopi: postgres.WopiRepository{}, Store: store, Guard: storage.NewUploadGuard(), Tokens: tokens,
		UnitOfWork: uow, Clock: clock.Fixed(now), IDs: generator{t}, SizeLimit: 1 << 20,
		Files: "https://hub.example/api/v1/wopi/files/",
	}
	controller := rest.NewRestController()
	controller.Wopi = &rest.WopiDoor{Host: host}
	server := httptest.NewServer(controller.Routes())
	defer server.Close()

	writer := mediaActor(tenantA, authorA)
	session, err := host.StartEditing(ctx, writer, object.ID)
	if err != nil {
		t.Fatal(err)
	}
	if session.WopiSrc != "https://hub.example/api/v1/wopi/files/"+object.ID.String() {
		t.Errorf("WOPI source %q", session.WopiSrc)
	}
	// The stub document server addresses the host it was given; here, the test's server.
	files := server.URL + rest.APIBasePath + "/wopi/files/" + object.ID.String()
	call := func(method, url, token string, header map[string]string, body []byte) answered {
		t.Helper()
		request, _ := http.NewRequestWithContext(ctx, method, url+"?access_token="+token, bytes.NewReader(body))
		for k, v := range header {
			request.Header.Set(k, v)
		}
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = response.Body.Close() }()
		read, _ := io.ReadAll(response.Body)
		return answered{StatusCode: response.StatusCode, Header: response.Header, Body: read}
	}

	// CheckFileInfo: the name, the size, and that this person may save.
	info := call(http.MethodGet, files, session.Token, nil, nil)
	var fileInfo map[string]any
	_ = json.Unmarshal(info.Body, &fileInfo)
	if info.StatusCode != http.StatusOK || fileInfo["BaseFileName"] != "plan.docx" ||
		fileInfo["UserCanWrite"] != true || fileInfo["Size"] != float64(len(docx)) {
		t.Fatalf("CheckFileInfo %d %v", info.StatusCode, fileInfo)
	}
	firstVersion := fileInfo["Version"]

	// GetFile: the bytes.
	got := call(http.MethodGet, files+"/contents", session.Token, nil, nil)
	if got.StatusCode != http.StatusOK || !bytes.Equal(got.Body, docx) {
		t.Fatalf("GetFile %d", got.StatusCode)
	}

	// A save without the lock is refused while the file holds bytes.
	saved := append(append([]byte{}, docx...), []byte("edited")...)
	if r := call(http.MethodPost, files+"/contents", session.Token, map[string]string{"X-WOPI-Override": "PUT", "X-WOPI-Lock": "s1"}, saved); r.StatusCode != http.StatusConflict {
		t.Fatalf("an unlocked save answered %d", r.StatusCode)
	}
	// Lock, then a second session's lock is refused with the holding one named.
	if r := call(http.MethodPost, files, session.Token, map[string]string{"X-WOPI-Override": "LOCK", "X-WOPI-Lock": "s1"}, nil); r.StatusCode != http.StatusOK {
		t.Fatalf("LOCK answered %d", r.StatusCode)
	}
	if r := call(http.MethodPost, files, session.Token, map[string]string{"X-WOPI-Override": "LOCK", "X-WOPI-Lock": "s2"}, nil); r.StatusCode != http.StatusConflict || r.Header.Get("X-WOPI-Lock") != "s1" {
		t.Fatalf("a second lock answered %d %q", r.StatusCode, r.Header.Get("X-WOPI-Lock"))
	}
	if r := call(http.MethodPost, files, session.Token, map[string]string{"X-WOPI-Override": "GET_LOCK"}, nil); r.Header.Get("X-WOPI-Lock") != "s1" {
		t.Fatalf("GET_LOCK answered %q", r.Header.Get("X-WOPI-Lock"))
	}

	// PutFile under the lock: a new version, the bytes it had kept as the earlier one.
	put := call(http.MethodPost, files+"/contents", session.Token, map[string]string{"X-WOPI-Override": "PUT", "X-WOPI-Lock": "s1"}, saved)
	if put.StatusCode != http.StatusOK || put.Header.Get("X-WOPI-ItemVersion") == "" || put.Header.Get("X-WOPI-ItemVersion") == firstVersion {
		t.Fatalf("PutFile %d version %q", put.StatusCode, put.Header.Get("X-WOPI-ItemVersion"))
	}
	again := call(http.MethodGet, files+"/contents", session.Token, nil, nil)
	if !bytes.Equal(again.Body, saved) {
		t.Fatal("GetFile after the save does not answer the saved bytes")
	}
	var versions []mediarepo.Version
	if err := read(ctx, t, tenantA, func(ctx context.Context) (err error) {
		versions, err = postgres.WopiRepository{}.Versions(ctx, object.ID)
		return err
	}); err != nil || len(versions) != 1 || versions[0].SavedBy != authorA {
		t.Fatalf("versions %+v, %v", versions, err)
	}
	earlier, err := store.Get(ctx, findMedia(ctx, t, tenantA, versions[0].EarlierMediaID).StorageKey)
	if err != nil {
		t.Fatal(err)
	}
	if read, _ := io.ReadAll(earlier.Content); !bytes.Equal(read, docx) {
		t.Error("the earlier version does not keep the bytes the file had")
	}
	_ = earlier.Content.Close()

	// Unlock with the wrong lock is refused; with the right one it is released.
	if r := call(http.MethodPost, files, session.Token, map[string]string{"X-WOPI-Override": "UNLOCK", "X-WOPI-Lock": "s2"}, nil); r.StatusCode != http.StatusConflict {
		t.Fatalf("a wrong UNLOCK answered %d", r.StatusCode)
	}
	if r := call(http.MethodPost, files, session.Token, map[string]string{"X-WOPI-Override": "UNLOCK", "X-WOPI-Lock": "s1"}, nil); r.StatusCode != http.StatusOK {
		t.Fatalf("UNLOCK answered %d", r.StatusCode)
	}

	// A reader's token: CheckFileInfo says read-only, and every write is refused.
	reader := tokens.Issue(wopi.Grant{TenantID: tenantA, AccountID: freshID(t), MediaID: object.ID, Write: true, ExpiresAt: now.Add(time.Hour)})
	info = call(http.MethodGet, files, reader, nil, nil)
	fileInfo = nil
	_ = json.Unmarshal(info.Body, &fileInfo)
	if fileInfo["UserCanWrite"] != false {
		t.Errorf("a reader may write: %v", fileInfo)
	}
	if r := call(http.MethodPost, files, reader, map[string]string{"X-WOPI-Override": "LOCK", "X-WOPI-Lock": "r"}, nil); r.StatusCode != http.StatusUnauthorized {
		t.Errorf("a reader's LOCK answered %d", r.StatusCode)
	}
	if r := call(http.MethodPost, files+"/contents", reader, map[string]string{"X-WOPI-Override": "PUT"}, saved); r.StatusCode != http.StatusUnauthorized {
		t.Errorf("a reader's PutFile answered %d", r.StatusCode)
	}

	// A forged token, and a token of another file, open nothing.
	if r := call(http.MethodGet, files, "forged", nil, nil); r.StatusCode != http.StatusUnauthorized {
		t.Errorf("a forged token answered %d", r.StatusCode)
	}
	other := tokens.Issue(wopi.Grant{TenantID: tenantA, AccountID: authorA, MediaID: freshID(t), Write: true, ExpiresAt: now.Add(time.Hour)})
	if r := call(http.MethodGet, files+"/contents", other, nil, nil); r.StatusCode != http.StatusUnauthorized {
		t.Errorf("another file's token answered %d", r.StatusCode)
	}
	// Another workspace's token for this file finds nothing there.
	foreign := tokens.Issue(wopi.Grant{TenantID: tenantB, AccountID: authorB, MediaID: object.ID, Write: true, ExpiresAt: now.Add(time.Hour)})
	if r := call(http.MethodGet, files, foreign, nil, nil); r.StatusCode != http.StatusNotFound {
		t.Errorf("another workspace's token answered %d", r.StatusCode)
	}

	// The viewer's question: the writer may edit, the reader may not.
	viewed, err := mediaservice.GetMedia{
		Objects: mediaRepo(), Containers: containerRepo(), Reader: writesOnly{writer: authorA},
		UnitOfWork: uow, Clock: clock.Fixed(now), Transfers: storage.NewLocalTransfers(security.NewMediaTokenIssuer(secret.New("an installation secret of decent length")), ""),
		Editing: host,
	}.Execute(ctx, writer, mediaservice.MediaQuery{MediaID: object.ID})
	if err != nil || !viewed.Editable {
		t.Errorf("the writer cannot edit: %+v %v", viewed.Editable, err)
	}
}
