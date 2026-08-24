// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	usecasemedia "github.com/Jersyfi/hubtask/core/application/service/media"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/storage"
)

var (
	mediaTenant = shared.MustParseID("0192f000-0000-7000-8000-00000000000a")
	mediaID     = shared.MustParseID("0192f000-0000-7000-8000-0000000000a1")
	mediaNow    = time.Date(2026, 8, 23, 9, 0, 0, 0, time.UTC)
)

// mediaTokens is the capability as this layer sees it: a signature it verifies and nothing it
// decides. What the real one signs is tested where it lives (infrastructure/security).
type mediaTokens struct {
	accept   string
	tenantID shared.ID
	err      error
	asked    []string
}

func (m *mediaTokens) ValidateUpload(token string, _ shared.ID, _ time.Time) (shared.ID, error) {
	return m.judge("upload:" + token)
}

func (m *mediaTokens) ValidateDownload(token string, _ shared.ID, _ time.Time) (shared.ID, error) {
	return m.judge("download:" + token)
}

func (m *mediaTokens) judge(asked string) (shared.ID, error) {
	m.asked = append(m.asked, asked)
	if m.err != nil {
		return "", m.err
	}
	if !strings.HasSuffix(asked, m.accept) {
		return "", shared.ErrValidation.WithDetail("media.token_invalid")
	}
	return m.tenantID, nil
}

type mediaContent struct {
	received []byte
	grant    usecasemedia.Grant
	served   usecasemedia.Served
	err      error
}

func (m *mediaContent) Receive(_ context.Context, grant usecasemedia.Grant, content io.Reader) error {
	m.grant = grant
	if m.err != nil {
		return m.err
	}
	body, err := io.ReadAll(content)
	m.received = body
	return err
}

func (m *mediaContent) Send(_ context.Context, grant usecasemedia.Grant) (usecasemedia.Served, error) {
	m.grant = grant
	if m.err != nil {
		return usecasemedia.Served{}, m.err
	}
	return m.served, nil
}

func mediaController(cat *catalogue, content *mediaContent, tokens *mediaTokens) *RestController {
	controller := NewRestController()
	controller.UseCases = cat
	controller.MediaContent = content
	controller.MediaTokens = tokens
	controller.Clock = clock.Fixed(mediaNow)
	return controller
}

func stagedProjection() usecase.Output {
	return usecase.Output{
		"id":           mediaID.String(),
		"file_name":    "plan.png",
		"content_type": "image/png",
		"size":         int64(32),
		"checksum":     nil,
		"usage":        "COVER",
		"status":       "PENDING",
		"ref_count":    0,
		"created_by":   mediaTenant.String(),
		"created_at":   mediaNow,
		"upload": map[string]any{
			"url": "https://storage.example/media/x", "method": "PUT",
			"expires_at": mediaNow.Add(15 * time.Minute),
		},
	}
}

func TestStagingAnUploadAnswersWithTheTargetAndItsLocation(t *testing.T) {
	cat := &catalogue{out: stagedProjection()}
	controller := mediaController(cat, &mediaContent{}, &mediaTokens{})

	body := `{"usage":"COVER","size":32,"file_name":"plan.png","content_type":"image/png"}`
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		APIBasePath+"/media", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	response := httptest.NewRecorder()
	controller.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status %d, body %s", response.Code, response.Body.String())
	}
	if cat.name != requestMediaUploadUseCase {
		t.Errorf("the controller invoked %q", cat.name)
	}
	if got := response.Header().Get("Location"); got != APIBasePath+"/media/"+mediaID.String() {
		t.Errorf("Location is %q", got)
	}

	var answered struct {
		Status string `json:"status"`
		Upload *struct {
			URL    string `json:"url"`
			Method string `json:"method"`
		} `json:"upload"`
		Download *struct{} `json:"download"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &answered); err != nil {
		t.Fatalf("the answer is not the schema's shape: %v", err)
	}
	if answered.Status != "PENDING" || answered.Upload == nil || answered.Upload.Method != "PUT" {
		t.Errorf("the answer is %+v", answered)
	}
	// A PENDING object has nowhere to download from, and the key is absent rather than null.
	if answered.Download != nil {
		t.Error("a staged object was given a download target")
	}
}

// The URL is the credential. A token minted for the other direction opens nothing, which is what
// keeps a download link from becoming a way to overwrite the bytes it points at.
func TestAContentTokenOpensOnlyItsOwnDirection(t *testing.T) {
	tokens := &mediaTokens{accept: "upload:secret", tenantID: mediaTenant}
	content := &mediaContent{}
	controller := mediaController(&catalogue{}, content, tokens)

	upload := httptest.NewRequestWithContext(t.Context(), http.MethodPut,
		APIBasePath+"/media/"+mediaID.String()+":content?token=secret",
		bytes.NewReader([]byte("the bytes")))
	response := httptest.NewRecorder()
	controller.Routes().ServeHTTP(response, upload)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status %d, body %s", response.Code, response.Body.String())
	}
	if string(content.received) != "the bytes" {
		t.Errorf("the handler passed on %q", content.received)
	}
	// The tenant comes out of the token and from nowhere else - there is nothing else on such a
	// request that could say it (multi-tenancy.md §2.2).
	if content.grant.TenantID != mediaTenant || content.grant.MediaID != mediaID {
		t.Errorf("the grant is %+v", content.grant)
	}

	download := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		APIBasePath+"/media/"+mediaID.String()+":content?token=secret", nil)
	response = httptest.NewRecorder()
	controller.Routes().ServeHTTP(response, download)

	if response.Code < 400 {
		t.Errorf("an upload token opened a download: status %d", response.Code)
	}
	if len(tokens.asked) != 2 || !strings.HasPrefix(tokens.asked[1], "download:") {
		t.Errorf("the handler asked about %v, want the download direction second", tokens.asked)
	}
}

// T-11: what comes back off a content route is a download and never a rendering path, whatever the
// bytes turned out to be.
func TestADownloadIsServedAsADownload(t *testing.T) {
	tokens := &mediaTokens{accept: "download:secret", tenantID: mediaTenant}
	content := &mediaContent{served: usecasemedia.Served{
		Content: storage.Object{
			Content:     io.NopCloser(strings.NewReader("<html>not rendered</html>")),
			Size:        25,
			ContentType: "text/plain; charset=utf-8",
		},
		FileName: `we"ird".html`,
	}}
	controller := mediaController(&catalogue{}, content, tokens)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		APIBasePath+"/media/"+mediaID.String()+":content?token=secret", nil)
	response := httptest.NewRecorder()
	controller.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", response.Code, response.Body.String())
	}
	if response.Body.String() != "<html>not rendered</html>" {
		t.Errorf("the bytes came back as %q", response.Body.String())
	}

	headers := map[string]string{
		// The stored type, which is the sniffed one - never anything the uploader claimed.
		"Content-Type":            "text/plain; charset=utf-8",
		"Content-Length":          "25",
		"X-Content-Type-Options":  "nosniff",
		"Content-Security-Policy": "sandbox; default-src 'none'",
		"Cache-Control":           "private, no-store",
	}
	for name, want := range headers {
		if got := response.Header().Get(name); got != want {
			t.Errorf("%s is %q, want %q", name, got, want)
		}
	}
	// The quoted-string form of RFC 6266 stays intact: a quote in the name would end the string
	// early and let the rest of it become another header parameter.
	if got := response.Header().Get("Content-Disposition"); got != `attachment; filename="weird.html"` {
		t.Errorf("Content-Disposition is %q", got)
	}
}

// An object with no name is still a download; the disposition simply carries no file name.
func TestADownloadWithoutAName(t *testing.T) {
	if got := dispositionFor(""); got != "attachment" {
		t.Errorf("the disposition is %q, want a bare attachment", got)
	}
}

// The content routes reach the application layer or they answer; they never half-run because the
// composition root forgot one of the three fields they need.
func TestTheContentRoutesRefuseWhenTheyAreNotWired(t *testing.T) {
	controller := NewRestController()

	request := httptest.NewRequestWithContext(t.Context(), http.MethodPut,
		APIBasePath+"/media/"+mediaID.String()+":content?token=secret", strings.NewReader("x"))
	response := httptest.NewRecorder()
	controller.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Errorf("status %d, want 500 - an unwired route is a defect, not a client's mistake", response.Code)
	}
}
