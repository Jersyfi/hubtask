// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package mailbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	httpport "github.com/Jersyfi/hubtask/core/port/httpclient"
	port "github.com/Jersyfi/hubtask/core/port/mailbox"
)

const (
	jmapCore    = "urn:ietf:params:jmap:core"
	jmapMail    = "urn:ietf:params:jmap:mail"
	targetClass = "mailbox"
)

// JMAP reads a mailbox over JMAP (RFC 8620, RFC 8621): JSON over HTTPS through the guarded client,
// so the egress allowlist and the address policy hold for it as for every other call out.
type JMAP struct {
	Client httpport.Port
}

var _ port.Reader = JMAP{}

type jmapSession struct {
	client   httpport.Port
	token    string
	apiURL   string
	download string
	account  string
	folder   string
	done     string
	doneID   string
	since    time.Time
	folderID string
}

// Open reads the session resource and finds the folder.
func (j JMAP) Open(ctx context.Context, connection port.Connection) (port.Session, error) {
	if j.Client == nil {
		return nil, shared.ErrInternal.WithDetail("mail.mailbox_unwired")
	}
	box := connection.Mailbox
	s := &jmapSession{
		client: j.Client, token: connection.Secret.Reveal(), folder: box.Folder, done: box.DoneFolder,
		since: box.CreatedAt,
	}
	var resource struct {
		APIURL          string            `json:"apiUrl"`
		DownloadURL     string            `json:"downloadUrl"`
		PrimaryAccounts map[string]string `json:"primaryAccounts"`
	}
	if err := s.get(ctx, box.SessionURL, &resource); err != nil {
		return nil, err
	}
	s.apiURL, s.download, s.account = resource.APIURL, resource.DownloadURL, resource.PrimaryAccounts[jmapMail]
	if s.apiURL == "" || s.download == "" || s.account == "" {
		return nil, shared.ErrUnavailable.WithDetail("mail.mailbox_unreachable").
			WithCause(errors.New("jmap: the session names no mail account"))
	}
	if err := s.findFolders(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

type mailboxRow struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

func (s *jmapSession) findFolders(ctx context.Context) error {
	var answer []json.RawMessage
	if err := s.call(ctx, [][]any{{"Mailbox/get", map[string]any{"accountId": s.account, "properties": []string{"id", "name", "role"}}, "m"}}, &answer); err != nil {
		return err
	}
	var got struct {
		List []mailboxRow `json:"list"`
	}
	if err := json.Unmarshal(answer[0], &got); err != nil {
		return unreadable(err)
	}
	for _, row := range got.List {
		if strings.EqualFold(s.folder, "INBOX") && row.Role == "inbox" || row.Name == s.folder {
			s.folderID = row.ID
		}
		if row.Name == s.done {
			s.doneID = row.ID
		}
	}
	if s.folderID == "" {
		return shared.ErrValidation.WithDetail("mail.mailbox_folder_missing").
			WithFields(shared.FieldError{Path: "/folder", Code: "mail.mailbox_folder_missing"})
	}
	return nil
}

// Fetch answers what arrived in the folder since the mailbox was connected, oldest first.
func (s *jmapSession) Fetch(ctx context.Context, limit int) ([]port.Message, error) {
	var answers []json.RawMessage
	err := s.call(ctx, [][]any{
		{"Email/query", map[string]any{
			"accountId": s.account,
			"filter":    map[string]any{"inMailbox": s.folderID, "after": s.since.UTC().Format(time.RFC3339)},
			"sort":      []any{map[string]any{"property": "receivedAt", "isAscending": true}},
			"limit":     limit,
		}, "q"},
		{"Email/get", map[string]any{
			"accountId":  s.account,
			"#ids":       map[string]any{"resultOf": "q", "name": "Email/query", "path": "/ids"},
			"properties": []string{"id", "blobId", "size"},
		}, "g"},
	}, &answers)
	if err != nil {
		return nil, err
	}
	var got struct {
		List []struct {
			ID     string `json:"id"`
			BlobID string `json:"blobId"`
			Size   int    `json:"size"`
		} `json:"list"`
	}
	if len(answers) < 2 || json.Unmarshal(answers[1], &got) != nil {
		return nil, unreadable(errors.New("jmap: Email/get answered nothing readable"))
	}
	var out []port.Message
	for _, email := range got.List {
		if email.Size > maxMessageBytes {
			continue
		}
		raw, err := s.blob(ctx, email.BlobID)
		if err != nil {
			return out, err
		}
		out = append(out, port.Message{Ref: email.ID, Raw: raw})
	}
	return out, nil
}

// Done files the message under the done folder alone, which takes it out of the inbox; the folder
// is made the first time.
func (s *jmapSession) Done(ctx context.Context, ref string) error {
	if s.doneID == "" {
		var answers []json.RawMessage
		if err := s.call(ctx, [][]any{{"Mailbox/set", map[string]any{
			"accountId": s.account, "create": map[string]any{"done": map[string]any{"name": s.done}},
		}, "c"}}, &answers); err != nil {
			return err
		}
		var created struct {
			Created map[string]struct {
				ID string `json:"id"`
			} `json:"created"`
		}
		if json.Unmarshal(answers[0], &created) != nil || created.Created["done"].ID == "" {
			return unreadable(errors.New("jmap: the done folder was not made"))
		}
		s.doneID = created.Created["done"].ID
	}
	var answers []json.RawMessage
	if err := s.call(ctx, [][]any{{"Email/set", map[string]any{
		"accountId": s.account,
		"update":    map[string]any{ref: map[string]any{"mailboxIds": map[string]bool{s.doneID: true}}},
	}, "u"}}, &answers); err != nil {
		return err
	}
	var updated struct {
		NotUpdated map[string]any `json:"notUpdated"`
	}
	if json.Unmarshal(answers[0], &updated) != nil || len(updated.NotUpdated) > 0 {
		return unreadable(fmt.Errorf("jmap: the message %s was not moved", ref))
	}
	return nil
}

func (s *jmapSession) Close() error { return nil }

func (s *jmapSession) get(ctx context.Context, address string, into any) error {
	response, err := s.client.Do(ctx, httpport.Request{
		Method: http.MethodGet, URL: address, TargetClass: targetClass,
		Header: map[string][]string{"Authorization": {"Bearer " + s.token}, "Accept": {"application/json"}},
	})
	if err != nil {
		return unreachable(err)
	}
	if err := statusOf(response.Status); err != nil {
		return err
	}
	if err := json.Unmarshal(response.Body, into); err != nil {
		return unreadable(err)
	}
	return nil
}

// call sends method calls and answers each method response's arguments, in order. A method error
// is a refusal of what was asked, said in the provider's type rather than its words.
func (s *jmapSession) call(ctx context.Context, calls [][]any, into *[]json.RawMessage) error {
	body, err := json.Marshal(map[string]any{"using": []string{jmapCore, jmapMail}, "methodCalls": calls})
	if err != nil {
		return shared.ErrInternal.WithCause(err)
	}
	response, err := s.client.Do(ctx, httpport.Request{
		Method: http.MethodPost, URL: s.apiURL, Body: body, TargetClass: targetClass,
		Header: map[string][]string{"Authorization": {"Bearer " + s.token}, "Content-Type": {"application/json"}},
	})
	if err != nil {
		return unreachable(err)
	}
	if err := statusOf(response.Status); err != nil {
		return err
	}
	var answer struct {
		MethodResponses [][]json.RawMessage `json:"methodResponses"`
	}
	if err := json.Unmarshal(response.Body, &answer); err != nil {
		return unreadable(err)
	}
	for _, each := range answer.MethodResponses {
		if len(each) < 2 {
			return unreadable(errors.New("jmap: a method response without arguments"))
		}
		var name string
		_ = json.Unmarshal(each[0], &name)
		if name == "error" {
			var failure struct {
				Type string `json:"type"`
			}
			_ = json.Unmarshal(each[1], &failure)
			return shared.ErrUnavailable.WithDetail("mail.mailbox_unreachable").
				WithCause(fmt.Errorf("jmap: method error %s", failure.Type))
		}
		*into = append(*into, each[1])
	}
	return nil
}

func (s *jmapSession) blob(ctx context.Context, blobID string) ([]byte, error) {
	address := strings.NewReplacer(
		"{accountId}", url.PathEscape(s.account), "{blobId}", url.PathEscape(blobID),
		"{type}", url.QueryEscape("message/rfc822"), "{name}", "message.eml",
	).Replace(s.download)
	response, err := s.client.Do(ctx, httpport.Request{
		Method: http.MethodGet, URL: address, TargetClass: targetClass,
		Header: map[string][]string{"Authorization": {"Bearer " + s.token}},
	})
	if err != nil {
		return nil, unreachable(err)
	}
	if err := statusOf(response.Status); err != nil {
		return nil, err
	}
	return response.Body, nil
}

func statusOf(status int) error {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return refused(fmt.Errorf("jmap: status %d", status))
	case status >= 300:
		return shared.ErrUnavailable.WithDetail("mail.mailbox_unreachable").WithCause(fmt.Errorf("jmap: status %d", status))
	}
	return nil
}

func unreadable(err error) error {
	return shared.ErrUnavailable.WithDetail("mail.mailbox_unreachable").WithCause(fmt.Errorf("jmap: %w", err))
}

// Readers chooses the reader by the mailbox's protocol.
type Readers struct {
	IMAP port.Reader
	JMAP port.Reader
}

var _ port.Reader = Readers{}

// Open opens the mailbox with the reader its protocol needs.
func (r Readers) Open(ctx context.Context, connection port.Connection) (port.Session, error) {
	if connection.Mailbox.Protocol == "JMAP" {
		return r.JMAP.Open(ctx, connection)
	}
	return r.IMAP.Open(ctx, connection)
}
