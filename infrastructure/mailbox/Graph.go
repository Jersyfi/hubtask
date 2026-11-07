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
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	httpport "github.com/Jersyfi/hubtask/core/port/httpclient"
	port "github.com/Jersyfi/hubtask/core/port/mailbox"
)

// graphBase is Microsoft Graph's v1.0 endpoint; a GRAPH mailbox names no server of its own.
const graphBase = "https://graph.microsoft.com/v1.0"

// Graph reads a Microsoft 365 or Outlook.com mailbox through Microsoft Graph, for a tenant whose
// Exchange administrator switched IMAP off: JSON over HTTPS through the guarded client, with the
// access token the mailbox's OAuth grant (Mail.ReadWrite) draws.
//
// The folder is listed rather than followed by a delta query: a filed message is moved out of it,
// so the folder itself is the cursor, and no delta link has to be kept between reads.
type Graph struct {
	Client httpport.Port
	// Base replaces graphBase in a test; empty is Microsoft's.
	Base string
}

var _ port.Reader = Graph{}

type graphSession struct {
	client   httpport.Port
	base     string
	token    string
	done     string
	doneID   string
	folderID string
	since    time.Time
}

type graphFolder struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

// Open finds the folder read and the done folder.
func (g Graph) Open(ctx context.Context, connection port.Connection) (port.Session, error) {
	if g.Client == nil {
		return nil, shared.ErrInternal.WithDetail("mail.mailbox_unwired")
	}
	box := connection.Mailbox
	s := &graphSession{
		client: g.Client, base: g.Base, token: connection.Secret.Reveal(), done: box.DoneFolder, since: box.CreatedAt,
	}
	if s.base == "" {
		s.base = graphBase
	}
	if strings.EqualFold(box.Folder, "INBOX") {
		var inbox graphFolder
		if err := s.send(ctx, http.MethodGet, "/me/mailFolders/inbox?$select=id,displayName", nil, &inbox); err != nil {
			return nil, err
		}
		s.folderID = inbox.ID
	} else {
		found, err := s.folderNamed(ctx, box.Folder)
		if err != nil {
			return nil, err
		}
		s.folderID = found
	}
	if s.folderID == "" {
		return nil, shared.ErrValidation.WithDetail("mail.mailbox_folder_missing").
			WithFields(shared.FieldError{Path: "/folder", Code: "mail.mailbox_folder_missing"})
	}
	doneID, err := s.folderNamed(ctx, s.done)
	if err != nil {
		return nil, err
	}
	s.doneID = doneID
	return s, nil
}

// folderNamed is a top-level folder by its display name, empty when there is none.
func (s *graphSession) folderNamed(ctx context.Context, name string) (string, error) {
	filter := "displayName eq '" + strings.ReplaceAll(name, "'", "''") + "'"
	var page struct {
		Value []graphFolder `json:"value"`
	}
	if err := s.send(ctx, http.MethodGet, "/me/mailFolders?$select=id,displayName&$filter="+url.QueryEscape(filter), nil, &page); err != nil {
		return "", err
	}
	for _, folder := range page.Value {
		if folder.DisplayName == name {
			return folder.ID, nil
		}
	}
	return "", nil
}

// Fetch answers what arrived in the folder since the mailbox was connected, oldest first, each as
// its MIME source.
func (s *graphSession) Fetch(ctx context.Context, limit int) ([]port.Message, error) {
	if limit < 1 {
		return nil, nil
	}
	query := url.Values{
		"$select":  {"id,receivedDateTime"},
		"$orderby": {"receivedDateTime asc"},
		"$filter":  {"receivedDateTime ge " + s.since.UTC().Format(time.RFC3339)},
		"$top":     {strconv.Itoa(limit)},
	}
	var page struct {
		Value []struct {
			ID string `json:"id"`
		} `json:"value"`
	}
	if err := s.send(ctx, http.MethodGet, "/me/mailFolders/"+url.PathEscape(s.folderID)+"/messages?"+query.Encode(), nil, &page); err != nil {
		return nil, err
	}
	var out []port.Message
	for _, message := range page.Value {
		raw, err := s.source(ctx, message.ID)
		if err != nil {
			return out, err
		}
		if raw == nil {
			continue
		}
		out = append(out, port.Message{Ref: message.ID, Raw: raw})
	}
	return out, nil
}

// source is a message's MIME source; nil for one larger than the intake takes, which stays.
func (s *graphSession) source(ctx context.Context, id string) ([]byte, error) {
	response, err := s.client.Do(ctx, httpport.Request{
		Method: http.MethodGet, URL: s.base + "/me/messages/" + url.PathEscape(id) + "/$value", TargetClass: targetClass,
		Header: map[string][]string{"Authorization": {"Bearer " + s.token}},
	})
	if err != nil {
		return nil, unreachable(err)
	}
	if err := graphStatus(response.Status); err != nil {
		return nil, err
	}
	if len(response.Body) > maxMessageBytes {
		return nil, nil
	}
	return response.Body, nil
}

// Done marks the message read and moves it to the done folder, made the first time.
func (s *graphSession) Done(ctx context.Context, ref string) error {
	if s.doneID == "" {
		var created graphFolder
		body, _ := json.Marshal(map[string]any{"displayName": s.done})
		if err := s.send(ctx, http.MethodPost, "/me/mailFolders", body, &created); err != nil {
			return err
		}
		if created.ID == "" {
			return graphUnreadable(errors.New("the done folder was not made"))
		}
		s.doneID = created.ID
	}
	read, _ := json.Marshal(map[string]any{"isRead": true})
	if err := s.send(ctx, http.MethodPatch, "/me/messages/"+url.PathEscape(ref), read, nil); err != nil {
		return err
	}
	move, _ := json.Marshal(map[string]any{"destinationId": s.doneID})
	var moved struct {
		ID string `json:"id"`
	}
	if err := s.send(ctx, http.MethodPost, "/me/messages/"+url.PathEscape(ref)+"/move", move, &moved); err != nil {
		return err
	}
	if moved.ID == "" {
		return graphUnreadable(fmt.Errorf("the message %s was not moved", ref))
	}
	return nil
}

func (s *graphSession) Close() error { return nil }

func (s *graphSession) send(ctx context.Context, method, path string, body []byte, into any) error {
	header := map[string][]string{"Authorization": {"Bearer " + s.token}, "Accept": {"application/json"}}
	if body != nil {
		header["Content-Type"] = []string{"application/json"}
	}
	response, err := s.client.Do(ctx, httpport.Request{
		Method: method, URL: s.base + path, Body: body, Header: header, TargetClass: targetClass,
	})
	if err != nil {
		return unreachable(err)
	}
	if err := graphStatus(response.Status); err != nil {
		return err
	}
	if into == nil || len(response.Body) == 0 {
		return nil
	}
	if err := json.Unmarshal(response.Body, into); err != nil {
		return graphUnreadable(err)
	}
	return nil
}

// graphStatus codes Graph's answer: a token it does not take, or a mailbox the grant does not
// reach, is a refusal; a folder that is gone is missing; anything else is the provider unreachable.
func graphStatus(status int) error {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return refused(fmt.Errorf("graph: status %d", status))
	case status == http.StatusNotFound:
		return shared.ErrValidation.WithDetail("mail.mailbox_folder_missing").
			WithFields(shared.FieldError{Path: "/folder", Code: "mail.mailbox_folder_missing"})
	case status >= 300:
		return shared.ErrUnavailable.WithDetail("mail.mailbox_unreachable").WithCause(fmt.Errorf("graph: status %d", status))
	}
	return nil
}

func graphUnreadable(err error) error {
	return shared.ErrUnavailable.WithDetail("mail.mailbox_unreachable").WithCause(fmt.Errorf("graph: %w", err))
}
