// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package filestore

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/filelink"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/filestore"
	httpport "github.com/Jersyfi/hubtask/core/port/httpclient"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

type store struct {
	answers  []httpport.Response
	requests []httpport.Request
}

func (s *store) Do(_ context.Context, req httpport.Request) (httpport.Response, error) {
	s.requests = append(s.requests, req)
	answer := s.answers[0]
	s.answers = s.answers[1:]
	return answer, nil
}

func answer(status int, body string) httpport.Response {
	return httpport.Response{Status: status, Body: []byte(body)}
}

// UC-INT-17 check 1: a Nextcloud's login flow is begun, polled until the person granted access,
// and a store that would send the poll elsewhere is not taken for a Nextcloud.
func TestANextcloudLoginFlowIsPolledUntilGranted(t *testing.T) {
	ctx := context.Background()
	fake := &store{answers: []httpport.Response{
		answer(200, `{"poll":{"token":"tok","endpoint":"https://cloud.example.org/login/v2/poll"},"login":"https://cloud.example.org/login/v2/flow/abc"}`),
		answer(404, ``),
		answer(200, `{"server":"https://cloud.example.org","loginName":"ada","appPassword":"app-pw"}`),
		answer(200, `{"poll":{"token":"tok","endpoint":"https://elsewhere.example.net/poll"},"login":"https://cloud.example.org/x"}`),
	}}
	s := Store{Client: fake}
	login, err := s.BeginLogin(ctx, "https://cloud.example.org")
	if err != nil || login.LoginURL != "https://cloud.example.org/login/v2/flow/abc" || login.PollToken.Reveal() != "tok" {
		t.Fatalf("begin = %+v %v", login, err)
	}
	if fake.requests[0].URL != "https://cloud.example.org/index.php/login/v2" || fake.requests[0].Header["User-Agent"][0] != "Hubtask" {
		t.Fatalf("begin asked %+v", fake.requests[0])
	}
	if _, granted, err := s.PollLogin(ctx, login.PollEndpoint, login.PollToken); granted || err != nil {
		t.Fatalf("not yet granted answered %v %v", granted, err)
	}
	grant, granted, err := s.PollLogin(ctx, login.PollEndpoint, login.PollToken)
	if !granted || err != nil || grant.Login != "ada" || grant.AppPassword.Reveal() != "app-pw" {
		t.Fatalf("grant = %+v %v %v", grant, granted, err)
	}
	if !strings.Contains(string(fake.requests[2].Body), "token=tok") {
		t.Fatalf("the poll carried %s", fake.requests[2].Body)
	}
	var e *shared.Error
	if _, err := s.BeginLogin(ctx, "https://cloud.example.org"); !errors.As(err, &e) || e.DetailCode != "filestore.not_a_nextcloud" {
		t.Fatalf("a poll elsewhere was taken: %v", err)
	}
}

const folder = `<?xml version="1.0"?>
<d:multistatus xmlns:d="DAV:" xmlns:oc="http://owncloud.org/ns">
 <d:response><d:href>/remote.php/dav/files/ada/Specs/</d:href>
  <d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype><oc:fileid>10</oc:fileid></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>
 <d:response><d:href>/remote.php/dav/files/ada/Specs/Login%20flow.pdf</d:href>
  <d:propstat><d:prop><d:displayname>Login flow.pdf</d:displayname><d:resourcetype/><d:getcontentlength>2048</d:getcontentlength>
   <d:getlastmodified>Fri, 30 Oct 2026 08:00:00 GMT</d:getlastmodified><oc:fileid>42</oc:fileid></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>
 <d:response><d:href>/remote.php/dav/files/ada/Specs/2026/</d:href>
  <d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype><oc:fileid>43</oc:fileid></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>
 <d:response><d:href>/remote.php/dav/files/bob/secret.txt</d:href>
  <d:propstat><d:prop><oc:fileid>99</oc:fileid></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>
</d:multistatus>`

// UC-INT-17 check 2: a Nextcloud folder answers its files and folders with the address an entry
// keeps, the folder itself and anything outside the person's own files left out.
func TestANextcloudFolderAnswersItsFilesAndFolders(t *testing.T) {
	fake := &store{answers: []httpport.Response{answer(http.StatusMultiStatus, folder)}}
	access := port.Access{Kind: filelink.ConnectNextcloud, Server: "https://cloud.example.org", Login: "ada", Secret: secret.New("app-pw")}
	items, err := Store{Client: fake}.Browse(context.Background(), access, "/Specs")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %+v", items)
	}
	file := items[0]
	if file.Ref != "/Specs/Login flow.pdf" || file.Name != "Login flow.pdf" || file.Folder || file.Size != 2048 ||
		file.Address != "https://cloud.example.org/index.php/f/42" || file.Provider != filelink.Nextcloud || file.Modified.IsZero() {
		t.Fatalf("file = %+v", file)
	}
	if !items[1].Folder || items[1].Ref != "/Specs/2026" {
		t.Fatalf("folder = %+v", items[1])
	}
	req := fake.requests[0]
	if req.Method != "PROPFIND" || req.URL != "https://cloud.example.org/remote.php/dav/files/ada/Specs" || req.Header["Depth"][0] != "1" ||
		!strings.HasPrefix(req.Header["Authorization"][0], "Basic ") {
		t.Fatalf("browse asked %+v", req)
	}
}

// UC-INT-17 check 3: a share link is read-only, made by OCS, and one that points elsewhere is not
// taken; a revoked app password is told as such.
func TestANextcloudShareIsReadOnly(t *testing.T) {
	fake := &store{answers: []httpport.Response{
		answer(200, `{"ocs":{"data":{"url":"https://cloud.example.org/s/aBcD"}}}`),
		answer(401, ``),
	}}
	access := port.Access{Kind: filelink.ConnectNextcloud, Server: "https://cloud.example.org", Login: "ada", Secret: secret.New("pw")}
	link, err := Store{Client: fake}.Share(context.Background(), access, "/Specs/Login flow.pdf")
	if err != nil || link != "https://cloud.example.org/s/aBcD" {
		t.Fatalf("share = %q %v", link, err)
	}
	body := string(fake.requests[0].Body)
	if !strings.Contains(body, "permissions=1") || !strings.Contains(body, "shareType=3") || fake.requests[0].Header["OCS-APIRequest"][0] != "true" {
		t.Fatalf("share asked %s", body)
	}
	var e *shared.Error
	if _, err := (Store{Client: fake}).Share(context.Background(), access, "/a"); !errors.As(err, &e) || e.DetailCode != "filestore.grant_revoked" {
		t.Fatalf("revoked = %v", err)
	}
}

// UC-INT-17 check 2: Microsoft's top is the person's OneDrive and the SharePoint sites they
// follow; a site answers its libraries and a library its files, a next page followed on Graph
// alone.
func TestMicrosoftFoldersAreOneDriveAndFollowedSites(t *testing.T) {
	fake := &store{answers: []httpport.Response{
		answer(200, `{"value":[{"id":"site-1","displayName":"Web team","webUrl":"https://contoso.sharepoint.com/sites/web"}]}`),
		answer(200, `{"value":[{"id":"b!lib","name":"Documents","webUrl":"https://contoso.sharepoint.com/sites/web/Shared%20Documents"}]}`),
		answer(200, `{"value":[{"id":"01F","name":"Plan","folder":{"childCount":2},"webUrl":"https://contoso.sharepoint.com/sites/web/Shared%20Documents/Plan"}],
		  "@odata.nextLink":"https://graph.microsoft.com/v1.0/drives/b!lib/items/root/children?$skiptoken=x"}`),
		answer(200, `{"value":[{"id":"01A","name":"Plan.docx","size":10,"webUrl":"https://contoso.sharepoint.com/sites/web/Shared%20Documents/Plan.docx"}],
		  "@odata.nextLink":"https://elsewhere.example.net/next"}`),
	}}
	access := port.Access{Kind: filelink.ConnectMicrosoft, Secret: secret.New("access")}
	s := Store{Client: fake}
	top, err := s.Browse(context.Background(), access, "")
	if err != nil || len(top) != 2 || top[0].Ref != "drive:me:root" || top[1].Ref != "site:site-1" || top[1].Name != "Web team" {
		t.Fatalf("top = %+v %v", top, err)
	}
	libraries, err := s.Browse(context.Background(), access, "site:site-1")
	if err != nil || len(libraries) != 1 || libraries[0].Ref != "drive:b!lib:root" {
		t.Fatalf("libraries = %+v %v", libraries, err)
	}
	files, err := s.Browse(context.Background(), access, "drive:b!lib:root")
	if err != nil || len(files) != 2 || !files[0].Folder || files[1].Folder || files[1].Provider != filelink.SharePoint {
		t.Fatalf("files = %+v %v", files, err)
	}
	if len(fake.requests) != 4 || fake.requests[0].Header["Authorization"][0] != "Bearer access" {
		t.Fatalf("requests = %d", len(fake.requests))
	}
}
