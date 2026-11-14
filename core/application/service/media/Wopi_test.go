// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package media

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/media"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
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
		Read: read, Wopi: chosenServer{}, Tokens: tokens, Pages: pages,
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
