// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package documentserver is the outbound adapter to a workspace's document server (UC-MED-05,
// ADR-0104): an office file converted to PDF for the viewer, through the server's own
// conversion - Collabora Online's convert-to, ONLYOFFICE's (and Euro-Office's) conversion API.
// Every call goes through GuardedClient with a deadline (rules 6, 7).
package documentserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/httpclient"
	"github.com/Jersyfi/hubtask/core/port/wopi"
	"github.com/Jersyfi/hubtask/infrastructure/httpclient"
)

// conversionDeadline bounds one conversion, the result's fetch included.
const conversionDeadline = 60 * time.Second

// targetClass labels the outbound metric; one value for every server (rule 10).
const targetClass = "document_server"

// Client is what this adapter needs of GuardedClient: a call with a small body and a call with a
// streamed one.
type Client interface {
	Do(ctx context.Context, req port.Request) (port.Response, error)
	Upload(ctx context.Context, req port.Request, body io.Reader, length int64) (port.Response, error)
}

// Server converts through the chosen document server.
type Server struct {
	Client Client
}

var _ wopi.Server = Server{}

// ConvertToPDF sends or points the server at the file and answers its PDF.
func (s Server) ConvertToPDF(
	ctx context.Context, choice media.DocumentServerChoice, source wopi.Source,
) (io.ReadCloser, error) {
	ctx, cancel := context.WithTimeout(ctx, conversionDeadline)
	defer cancel()
	switch choice.Kind {
	case media.DocumentServerCollabora:
		return s.collabora(ctx, choice.URL, source)
	case media.DocumentServerOnlyOffice, media.DocumentServerEuroOffice:
		return s.onlyOffice(ctx, choice.URL, source)
	default:
		return nil, wopi.ErrUnsupported()
	}
}

// reachDeadline bounds the administration's test of a server.
const reachDeadline = 10 * time.Second

// Reach asks <origin>/hosting/discovery, where Collabora Online, ONLYOFFICE and Euro-Office
// answer their WOPI discovery.
func (s Server) Reach(ctx context.Context, choice media.DocumentServerChoice) error {
	ctx, cancel := context.WithTimeout(ctx, reachDeadline)
	defer cancel()
	response, err := s.Client.Do(ctx, port.Request{
		Method: http.MethodGet, URL: choice.URL + "/hosting/discovery", TargetClass: targetClass,
		Header: map[string][]string{"Accept": {"application/xml, text/xml"}},
	})
	switch {
	case httpclient.IsPrivateTarget(err):
		return shared.ErrValidation.WithDetail("media.document_server_private_network").WithCause(err)
	case httpclient.IsBlocked(err):
		return err
	case err != nil:
		return shared.ErrUnavailable.WithDetail("media.document_server_unreachable").WithCause(err)
	}
	if response.Status != http.StatusOK || !bytes.Contains(response.Body, []byte("<wopi-discovery")) {
		return shared.ErrUnavailable.WithDetail("media.document_server_no_discovery").
			WithCause(fmt.Errorf("the discovery answered %d", response.Status))
	}
	return nil
}

// collabora posts the file as a form to /cool/convert-to/pdf and reads the PDF back.
func (s Server) collabora(ctx context.Context, origin string, source wopi.Source) (io.ReadCloser, error) {
	var head bytes.Buffer
	form := multipart.NewWriter(&head)
	part := textproto.MIMEHeader{}
	part.Set("Content-Disposition", `form-data; name="data"; filename="`+safeName(source.Name)+`"`)
	part.Set("Content-Type", "application/octet-stream")
	if _, err := form.CreatePart(part); err != nil {
		return nil, failed(err)
	}
	tail := "\r\n--" + form.Boundary() + "--\r\n"
	body := io.MultiReader(bytes.NewReader(head.Bytes()), source.Content, strings.NewReader(tail))
	length := int64(head.Len()) + source.Size + int64(len(tail))

	response, err := s.Client.Upload(ctx, port.Request{
		Method: http.MethodPost, URL: origin + "/cool/convert-to/pdf", TargetClass: targetClass,
		Header: map[string][]string{"Content-Type": {form.FormDataContentType()}},
	}, body, length)
	if err != nil {
		return nil, err
	}
	return pdfFrom(response)
}

// onlyOffice asks the conversion API to fetch the file through GetFile and convert it, then
// fetches the result it names.
func (s Server) onlyOffice(ctx context.Context, origin string, source wopi.Source) (io.ReadCloser, error) {
	ending := strings.TrimPrefix(strings.ToLower(path.Ext(source.Name)), ".")
	request, err := json.Marshal(map[string]any{
		"async": false, "filetype": ending, "key": source.Key, "outputtype": "pdf",
		"title": safeName(source.Name), "url": source.FetchAt,
	})
	if err != nil {
		return nil, failed(err)
	}
	response, err := s.Client.Do(ctx, port.Request{
		Method: http.MethodPost, URL: origin + "/ConvertService.ashx", TargetClass: targetClass,
		Header: map[string][]string{"Content-Type": {"application/json"}, "Accept": {"application/json"}},
		Body:   request,
	})
	if err != nil {
		return nil, err
	}
	if response.Status != http.StatusOK {
		return nil, refused(response.Status)
	}
	var answer struct {
		EndConvert bool   `json:"endConvert"`
		FileURL    string `json:"fileUrl"`
		Error      int    `json:"error"`
	}
	if err := json.Unmarshal(response.Body, &answer); err != nil || answer.Error != 0 || !answer.EndConvert || answer.FileURL == "" {
		return nil, refused(response.Status)
	}
	// The result lives on the server that converted it; anything else is not this server's answer.
	if !strings.HasPrefix(answer.FileURL, origin+"/") {
		return nil, refused(response.Status)
	}
	result, err := s.Client.Do(ctx, port.Request{Method: http.MethodGet, URL: answer.FileURL, TargetClass: targetClass})
	if err != nil {
		return nil, err
	}
	return pdfFrom(result)
}

// pdfFrom checks that the server answered a PDF; anything else is never handed to the viewer.
func pdfFrom(response port.Response) (io.ReadCloser, error) {
	if response.Status != http.StatusOK || !bytes.HasPrefix(response.Body, []byte("%PDF-")) {
		return nil, refused(response.Status)
	}
	return io.NopCloser(bytes.NewReader(response.Body)), nil
}

// safeName keeps a file name inside a quoted form value: quotes, backslashes and line breaks go.
func safeName(name string) string {
	return strings.Map(func(r rune) rune {
		if r == '"' || r == '\\' || r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, name)
}

func refused(status int) error {
	return shared.ErrUnavailable.WithDetail("dependency.document_server_refused").
		WithCause(fmt.Errorf("the document server answered %d", status))
}

func failed(err error) error {
	return shared.ErrInternal.WithDetail("media.conversion_failed").WithCause(err)
}
