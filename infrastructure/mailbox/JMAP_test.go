// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package mailbox

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	httpport "github.com/Jersyfi/hubtask/core/port/httpclient"
	port "github.com/Jersyfi/hubtask/core/port/mailbox"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// provider is a JMAP server behind the HTTP port, as Fastmail answers.
type provider struct {
	token    string
	hasDone  bool
	moved    map[string]string
	queries  []map[string]any
	notMoved bool
}

func (p *provider) Do(_ context.Context, request httpport.Request) (httpport.Response, error) {
	if request.Header["Authorization"][0] != "Bearer "+p.token {
		return httpport.Response{Status: http.StatusUnauthorized}, nil
	}
	answer := func(body any) (httpport.Response, error) {
		encoded, _ := json.Marshal(body)
		return httpport.Response{Status: http.StatusOK, Body: encoded}, nil
	}
	switch {
	case strings.HasSuffix(request.URL, "/session"):
		return answer(map[string]any{
			"apiUrl": "https://jmap.test/api", "downloadUrl": "https://jmap.test/download/{accountId}/{blobId}/{name}?type={type}",
			"primaryAccounts": map[string]string{jmapMail: "acc1"},
		})
	case strings.HasPrefix(request.URL, "https://jmap.test/download/"):
		blob := strings.Split(strings.TrimPrefix(request.URL, "https://jmap.test/download/acc1/"), "/")[0]
		return httpport.Response{Status: http.StatusOK, Body: []byte("Subject: " + blob + "\r\n\r\nbody")}, nil
	}
	var call struct {
		MethodCalls [][]json.RawMessage `json:"methodCalls"`
	}
	_ = json.Unmarshal(request.Body, &call)
	var responses []any
	for _, each := range call.MethodCalls {
		var name, id string
		var arguments map[string]any
		_ = json.Unmarshal(each[0], &name)
		_ = json.Unmarshal(each[1], &arguments)
		_ = json.Unmarshal(each[2], &id)
		switch name {
		case "Mailbox/get":
			list := []map[string]string{{"id": "in", "name": "Inbox", "role": "inbox"}}
			if p.hasDone {
				list = append(list, map[string]string{"id": "done", "name": "Hubtask"})
			}
			responses = append(responses, []any{name, map[string]any{"list": list}, id})
		case "Email/query":
			p.queries = append(p.queries, arguments)
			responses = append(responses, []any{name, map[string]any{"ids": []string{"e1", "e2"}}, id})
		case "Email/get":
			responses = append(responses, []any{name, map[string]any{"list": []map[string]any{
				{"id": "e1", "blobId": "b1", "size": 100}, {"id": "e2", "blobId": "b2", "size": maxMessageBytes + 1},
			}}, id})
		case "Mailbox/set":
			p.hasDone = true
			responses = append(responses, []any{name, map[string]any{"created": map[string]any{"done": map[string]string{"id": "done"}}}, id})
		case "Email/set":
			if p.notMoved {
				responses = append(responses, []any{name, map[string]any{"notUpdated": map[string]any{"e1": map[string]string{"type": "forbidden"}}}, id})
				continue
			}
			for ref, change := range arguments["update"].(map[string]any) {
				for box := range change.(map[string]any)["mailboxIds"].(map[string]any) {
					p.moved[ref] = box
				}
			}
			responses = append(responses, []any{name, map[string]any{"updated": map[string]any{}}, id})
		default:
			responses = append(responses, []any{"error", map[string]string{"type": "unknownMethod"}, id})
		}
	}
	return answer(map[string]any{"methodResponses": responses})
}

func jmapConnection(token string) port.Connection {
	box, err := domain.NewMailbox(domain.NewMailboxInput{ID: "m2", TenantID: "t1", Preset: "fastmail", Address: "familie@fastmail.com", Now: connected})
	if err != nil {
		panic(err)
	}
	box.SessionURL = "https://jmap.test/session"
	return port.Connection{Mailbox: box, Secret: secret.New(token)}
}

func TestAJMAPMailboxIsReadSinceConnectingAndMovedToItsFolder(t *testing.T) {
	server := &provider{token: "tok", moved: map[string]string{}}
	reader := Readers{JMAP: JMAP{Client: server}}
	session, err := reader.Open(context.Background(), jmapConnection("tok"))
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	messages, err := session.Fetch(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Ref != "e1" || !strings.Contains(string(messages[0].Raw), "Subject: b1") {
		t.Fatalf("fetched %+v (a message past the size bound must stay)", messages)
	}
	filter := server.queries[0]["filter"].(map[string]any)
	if filter["inMailbox"] != "in" || filter["after"] != "2026-10-25T10:00:00Z" {
		t.Errorf("queried %+v", filter)
	}
	if err := session.Done(context.Background(), "e1"); err != nil {
		t.Fatal(err)
	}
	if server.moved["e1"] != "done" || !server.hasDone {
		t.Errorf("moved %+v", server.moved)
	}
	server.notMoved = true
	if err := session.Done(context.Background(), "e1"); err == nil {
		t.Error("a move the server refused was taken as done")
	}
}

func TestAJMAPTokenThatDoesNotWorkIsARefusal(t *testing.T) {
	_, err := (JMAP{Client: &provider{token: "right", moved: map[string]string{}}}).Open(context.Background(), jmapConnection("wrong"))
	if shared.AsError(err).DetailCode != "mail.mailbox_refused" {
		t.Errorf("a wrong token answered %v", err)
	}
	if _, err := (JMAP{}).Open(context.Background(), jmapConnection("x")); err == nil {
		t.Error("an unwired reader opened")
	}
	missing := jmapConnection("right")
	missing.Mailbox.Folder = "Nirgends"
	if _, err := (JMAP{Client: &provider{token: "right", moved: map[string]string{}}}).Open(context.Background(), missing); shared.AsError(err).DetailCode != "mail.mailbox_folder_missing" {
		t.Errorf("a missing folder answered %v", err)
	}
}
