// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package documentserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	port "github.com/Jersyfi/hubtask/core/port/httpclient"
	"github.com/Jersyfi/hubtask/core/port/wopi"
	"github.com/Jersyfi/hubtask/infrastructure/httpclient"
)

// plain stands in for GuardedClient against a local stub, through the stub's own client: the
// guard itself is GuardedClient's test's subject, the request this adapter sends is this one's.
type plain struct{ client *http.Client }

func (p plain) Do(ctx context.Context, req port.Request) (port.Response, error) {
	return p.Upload(ctx, req, bytes.NewReader(req.Body), int64(len(req.Body)))
}

func (p plain) Upload(ctx context.Context, req port.Request, body io.Reader, length int64) (port.Response, error) {
	r, err := http.NewRequestWithContext(ctx, req.Method, req.URL, body)
	if err != nil {
		return port.Response{}, err
	}
	r.ContentLength = length
	for k, v := range req.Header {
		r.Header[k] = v
	}
	answer, err := p.client.Do(r)
	if err != nil {
		return port.Response{}, err
	}
	defer func() { _ = answer.Body.Close() }()
	read, err := io.ReadAll(answer.Body)
	return port.Response{Status: answer.StatusCode, Header: answer.Header, Body: read}, err
}

const pdf = "%PDF-1.7 converted"

func TestCollaboraConvertsTheSentFile(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cool/convert-to/pdf" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		file, header, err := r.FormFile("data")
		if err != nil || header.Filename != "plan.docx" {
			http.Error(w, "no file", http.StatusBadRequest)
			return
		}
		if read, _ := io.ReadAll(file); string(read) != "the document" {
			http.Error(w, "wrong bytes", http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, pdf)
	}))
	defer stub.Close()

	out, err := Server{Client: plain{stub.Client()}}.ConvertToPDF(context.Background(),
		media.DocumentServerChoice{Kind: media.DocumentServerCollabora, URL: stub.URL},
		wopi.Source{Name: `plan.docx`, Content: strings.NewReader("the document"), Size: 12})
	if err != nil {
		t.Fatal(err)
	}
	if read, _ := io.ReadAll(out); string(read) != pdf {
		t.Fatalf("read %q", read)
	}
}

func TestOnlyOfficeFetchesThroughGetFileAndAnswersTheResult(t *testing.T) {
	var stub *httptest.Server
	stub = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ConvertService.ashx":
			var asked map[string]any
			_ = json.NewDecoder(r.Body).Decode(&asked)
			if asked["url"] != "https://hub.example/api/v1/wopi/files/f/contents?access_token=t" ||
				asked["outputtype"] != "pdf" || asked["filetype"] != "xlsx" || asked["key"] != "k1" {
				http.Error(w, "wrong request", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"endConvert": true, "fileUrl": stub.URL + "/cache/out.pdf"})
		case "/cache/out.pdf":
			_, _ = io.WriteString(w, pdf)
		default:
			http.NotFound(w, r)
		}
	}))
	defer stub.Close()

	out, err := Server{Client: plain{stub.Client()}}.ConvertToPDF(context.Background(),
		media.DocumentServerChoice{Kind: media.DocumentServerOnlyOffice, URL: stub.URL},
		wopi.Source{Key: "k1", Name: "sheet.xlsx", FetchAt: "https://hub.example/api/v1/wopi/files/f/contents?access_token=t"})
	if err != nil {
		t.Fatal(err)
	}
	if read, _ := io.ReadAll(out); string(read) != pdf {
		t.Fatalf("read %q", read)
	}
}

func TestAConversionThatIsNoPDFIsRefused(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "<html>not a pdf</html>")
	}))
	defer stub.Close()
	_, err := Server{Client: plain{stub.Client()}}.ConvertToPDF(context.Background(),
		media.DocumentServerChoice{Kind: media.DocumentServerCollabora, URL: stub.URL},
		wopi.Source{Name: "a.docx", Content: strings.NewReader("x"), Size: 1})
	if !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("answered %v", err)
	}
	_, err = Server{Client: plain{stub.Client()}}.ConvertToPDF(context.Background(),
		media.DocumentServerChoice{Kind: media.DocumentServerMicrosoft, URL: stub.URL}, wopi.Source{})
	if shared.AsError(err).DetailCode != "media.conversion_unavailable" {
		t.Fatalf("the largest vendor's office converted: %v", err)
	}
}

// UC-MED-06 check 1: the administration's test of a server asks its WOPI discovery and tells
// apart a server that answers, something that answers but is no document server, nothing at all,
// and an address inside a private network, which the outbound guard refuses before any call.
func TestADocumentServerIsReachedThroughItsDiscovery(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/hosting/discovery":
			_, _ = io.WriteString(w, `<?xml version="1.0"?><wopi-discovery><net-zone/></wopi-discovery>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer stub.Close()
	welcome := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "<html>welcome</html>")
	}))
	defer welcome.Close()
	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()

	server := Server{Client: plain{client: stub.Client()}}
	for _, kind := range []media.DocumentServer{media.DocumentServerCollabora, media.DocumentServerOnlyOffice, media.DocumentServerEuroOffice} {
		if err := server.Reach(t.Context(), media.DocumentServerChoice{Kind: kind, URL: stub.URL}); err != nil {
			t.Errorf("%s: %v", kind, err)
		}
	}
	for _, c := range []struct {
		name, url, code string
	}{
		{"a web page", welcome.URL, "media.document_server_no_discovery"},
		{"nothing listening", gone.URL, "media.document_server_unreachable"},
	} {
		err := server.Reach(t.Context(), media.DocumentServerChoice{Kind: media.DocumentServerCollabora, URL: c.url})
		if detail(err) != c.code {
			t.Errorf("%s: got %v, want %s", c.name, err, c.code)
		}
	}

	guarded := Server{Client: httpclient.NewGuardedClient(env.OutboundConfig{Timeout: time.Second}, httpclient.NewGuard(env.OutboundConfig{}))}
	err := guarded.Reach(t.Context(), media.DocumentServerChoice{Kind: media.DocumentServerCollabora, URL: stub.URL})
	if detail(err) != "media.document_server_private_network" {
		t.Errorf("a server on a private network: got %v", err)
	}
}

func detail(err error) string {
	var typed *shared.Error
	if errors.As(err, &typed) {
		return typed.DetailCode
	}
	return ""
}

// UC-MED-06 check 2: the editor's address comes from the server's discovery - the edit action for
// the file's ending, its placeholders dropped - and only from the server's own origin.
func TestTheEditorsAddressComesFromTheDiscovery(t *testing.T) {
	var origin string
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hosting/discovery" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, `<?xml version="1.0"?><wopi-discovery><net-zone name="external-http">`+
			`<app name="writer"><action name="view" ext="docx" urlsrc="`+origin+`/view/docx?"/>`+
			`<action name="edit" ext="docx" urlsrc="`+origin+`/browser/abc/cool.html?&lt;ui=UI_LLCC&amp;&gt;"/></app>`+
			`<app name="calc"><action name="edit" ext="xlsx" urlsrc="https://elsewhere.example/calc?"/></app>`+
			`</net-zone></wopi-discovery>`)
	}))
	defer stub.Close()
	origin = stub.URL
	server := Server{Client: plain{client: stub.Client()}}
	choice := media.DocumentServerChoice{Kind: media.DocumentServerCollabora, URL: stub.URL}

	action, err := server.EditAction(t.Context(), choice, "Angebot.DOCX")
	if err != nil || action != stub.URL+"/browser/abc/cool.html?" {
		t.Fatalf("edit action %q, %v", action, err)
	}
	if _, err := server.EditAction(t.Context(), choice, "budget.xlsx"); detail(err) != "media.document_server_no_editor" {
		t.Errorf("an action on another origin: %v", err)
	}
	if _, err := server.EditAction(t.Context(), choice, "notes.odt"); detail(err) != "media.document_server_no_editor" {
		t.Errorf("an ending the server does not edit: %v", err)
	}
}
