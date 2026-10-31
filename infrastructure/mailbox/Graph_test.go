// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package mailbox

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	httpport "github.com/Jersyfi/hubtask/core/port/httpclient"
	port "github.com/Jersyfi/hubtask/core/port/mailbox"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// exchange answers as Microsoft Graph v1.0 does for a mailbox; the bodies are trimmed recordings
// of its answers (2026-10), the ids shortened.
type exchange struct {
	token     string
	hasDone   bool
	requests  []string
	read      []string
	moved     map[string]string
	listQuery url.Values
}

const (
	recordedInbox  = `{"@odata.context":"https://graph.microsoft.com/v1.0/$metadata#users('x')/mailFolders/$entity","id":"AAMkInbox","displayName":"Inbox"}`
	recordedFolder = `{"@odata.context":"https://graph.microsoft.com/v1.0/$metadata#users('x')/mailFolders(id,displayName)","value":[{"id":"AAMkHubtask","displayName":"Hubtask"}]}`
	recordedNone   = `{"@odata.context":"https://graph.microsoft.com/v1.0/$metadata#users('x')/mailFolders(id,displayName)","value":[]}`
	recordedList   = `{"@odata.context":"https://graph.microsoft.com/v1.0/$metadata#users('x')/mailFolders('AAMkInbox')/messages(id,receivedDateTime)","value":[` +
		`{"@odata.etag":"W/\"CQAAABYAAAA\"","id":"AAMkM1","receivedDateTime":"2026-10-30T08:00:00Z"},` +
		`{"@odata.etag":"W/\"CQAAABYAAAB\"","id":"AAMkM2","receivedDateTime":"2026-10-30T09:00:00Z"}]}`
)

func (e *exchange) Do(_ context.Context, request httpport.Request) (httpport.Response, error) {
	e.requests = append(e.requests, request.Method+" "+request.URL)
	if request.Header["Authorization"][0] != "Bearer "+e.token {
		return httpport.Response{Status: http.StatusUnauthorized, Body: []byte(`{"error":{"code":"InvalidAuthenticationToken"}}`)}, nil
	}
	ok := func(body string) (httpport.Response, error) {
		return httpport.Response{Status: http.StatusOK, Body: []byte(body)}, nil
	}
	path := strings.TrimPrefix(request.URL, graphBase)
	switch {
	case request.Method == http.MethodGet && strings.HasPrefix(path, "/me/mailFolders/inbox"):
		return ok(recordedInbox)
	case request.Method == http.MethodGet && strings.HasPrefix(path, "/me/mailFolders?"):
		if e.hasDone {
			return ok(recordedFolder)
		}
		return ok(recordedNone)
	case request.Method == http.MethodPost && path == "/me/mailFolders":
		e.hasDone = true
		return httpport.Response{Status: http.StatusCreated, Body: []byte(`{"id":"AAMkHubtask","displayName":"Hubtask"}`)}, nil
	case request.Method == http.MethodGet && strings.HasPrefix(path, "/me/mailFolders/AAMkInbox/messages?"):
		parsed, _ := url.Parse(request.URL)
		e.listQuery = parsed.Query()
		return ok(recordedList)
	case request.Method == http.MethodGet && strings.HasSuffix(path, "/$value"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/me/messages/"), "/$value")
		return httpport.Response{Status: http.StatusOK, Body: []byte("Subject: " + id + "\r\n\r\nbody")}, nil
	case request.Method == http.MethodPatch:
		var body map[string]any
		_ = json.Unmarshal(request.Body, &body)
		if body["isRead"] == true {
			e.read = append(e.read, strings.TrimPrefix(path, "/me/messages/"))
		}
		return ok(`{"id":"x","isRead":true}`)
	case request.Method == http.MethodPost && strings.HasSuffix(path, "/move"):
		var body map[string]string
		_ = json.Unmarshal(request.Body, &body)
		e.moved[strings.TrimSuffix(strings.TrimPrefix(path, "/me/messages/"), "/move")] = body["destinationId"]
		return httpport.Response{Status: http.StatusCreated, Body: []byte(`{"id":"AAMkMoved"}`)}, nil
	}
	return httpport.Response{Status: http.StatusNotFound}, nil
}

func graphBox(t *testing.T) domain.Mailbox {
	t.Helper()
	box, err := domain.NewMailbox(domain.NewMailboxInput{
		ID: "g1", TenantID: "t1", Protocol: domain.ProtocolGraph,
		Address: "desk@contoso.example", Now: time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return box
}

// A Microsoft 365 mailbox with IMAP switched off is read through Graph: the inbox oldest first
// since it was connected, each message's MIME source, and once filed it is marked read and moved to
// the done folder, which is made the first time.
func TestGraphReadsTheInboxAndFilesWhatItTook(t *testing.T) {
	provider := &exchange{token: "access", moved: map[string]string{}}
	session, err := Graph{Client: provider}.Open(context.Background(), port.Connection{Mailbox: graphBox(t), Secret: secret.New("access")})
	if err != nil {
		t.Fatal(err)
	}
	messages, err := session.Fetch(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Ref != "AAMkM1" || !strings.Contains(string(messages[1].Raw), "Subject: AAMkM2") {
		t.Fatalf("messages = %+v", messages)
	}
	if got := provider.listQuery.Get("$filter"); got != "receivedDateTime ge 2026-10-29T00:00:00Z" {
		t.Errorf("filter = %q, want the connection's time", got)
	}
	if provider.listQuery.Get("$orderby") != "receivedDateTime asc" || provider.listQuery.Get("$top") != "10" {
		t.Errorf("query = %v", provider.listQuery)
	}
	if err := session.Done(context.Background(), "AAMkM1"); err != nil {
		t.Fatal(err)
	}
	if err := session.Done(context.Background(), "AAMkM2"); err != nil {
		t.Fatal(err)
	}
	if provider.moved["AAMkM1"] != "AAMkHubtask" || provider.moved["AAMkM2"] != "AAMkHubtask" || len(provider.read) != 2 {
		t.Errorf("moved = %v, read = %v", provider.moved, provider.read)
	}
	created := 0
	for _, request := range provider.requests {
		if request == http.MethodPost+" "+graphBase+"/me/mailFolders" {
			created++
		}
	}
	if created != 1 {
		t.Errorf("the done folder was made %d times", created)
	}
	for _, request := range provider.requests {
		if !strings.HasPrefix(request, http.MethodGet+" "+graphBase) && !strings.HasPrefix(request, http.MethodPost+" "+graphBase) &&
			!strings.HasPrefix(request, http.MethodPatch+" "+graphBase) {
			t.Errorf("a call left Graph: %s", request)
		}
	}
}

// A token Graph does not take is the mailbox refusing, said in a code - so the administrator is
// told to connect again rather than that the server is down.
func TestGraphRefusalIsCoded(t *testing.T) {
	provider := &exchange{token: "access", moved: map[string]string{}}
	_, err := Graph{Client: provider}.Open(context.Background(), port.Connection{Mailbox: graphBox(t), Secret: secret.New("stale")})
	var coded *shared.Error
	if !errors.As(err, &coded) || coded.DetailCode != "mail.mailbox_refused" {
		t.Fatalf("err = %v", err)
	}
}

// The readers send a GRAPH mailbox to Graph and nowhere else.
func TestReadersSendGraphToGraph(t *testing.T) {
	provider := &exchange{token: "access", moved: map[string]string{}}
	readers := Readers{IMAP: Graph{}, JMAP: Graph{}, Graph: Graph{Client: provider}}
	if _, err := readers.Open(context.Background(), port.Connection{Mailbox: graphBox(t), Secret: secret.New("access")}); err != nil {
		t.Fatal(err)
	}
	if _, err := (Readers{}).Open(context.Background(), port.Connection{Mailbox: graphBox(t)}); err == nil {
		t.Error("a GRAPH mailbox opened without a Graph reader")
	}
}
