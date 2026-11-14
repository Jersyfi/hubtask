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

	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/httpclient"
	"github.com/Jersyfi/hubtask/core/port/wopi"
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
